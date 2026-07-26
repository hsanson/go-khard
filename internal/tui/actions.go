package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/config"
	"github.com/hsanson/go-khard/internal/contact"
)

func (m *model) current() *contact.Contact {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return &m.visible[m.cursor]
}
func (m *model) targets() []contact.Contact {
	var out []contact.Contact
	for _, c := range m.contacts {
		if m.selected[c.Path] {
			out = append(out, c)
		}
	}
	if len(out) == 0 && m.current() != nil {
		out = append(out, *m.current())
	}
	return out
}
func (m *model) selectedCount() int {
	n := 0
	for _, v := range m.selected {
		if v {
			n++
		}
	}
	return n
}
func (m *model) pageSize() int { return max(1, m.height-6) }
func (m *model) clamp() {
	if len(m.visible) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	m.cursor = max(0, min(m.cursor, len(m.visible)-1))
	page := m.pageSize()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+page {
		m.offset = m.cursor - page + 1
	}
}
func (m *model) filter() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	m.visible = nil
	for _, c := range m.contacts {
		if q == "" || fuzzy(c.SearchText(), q) {
			m.visible = append(m.visible, c)
		}
	}
	m.cursor = 0
	m.offset = 0
}
func fuzzy(text, q string) bool {
	i := 0
	query := []rune(q)
	for _, r := range text {
		if i < len(query) && r == query[i] {
			i++
		}
	}
	return i == len(query)
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func (m *model) startBookOperation(op operation) {
	if len(m.targets()) == 0 {
		m.message = "no contact"
		return
	}
	if len(m.books) == 0 {
		m.message = "no addressbooks configured"
		return
	}
	m.op = op
	m.bookCursor = 0
	m.mode = modeBooks
}
func (m *model) startConfirmation(op operation) {
	if len(m.targets()) == 0 {
		m.message = "no contact"
		return
	}
	m.op = op
	m.mode = modeConfirm
}
func (m *model) confirmText() string {
	n := len(m.targets())
	switch m.op {
	case opCopy:
		return fmt.Sprintf("Copy %d contact(s) to %s?", n, m.books[m.bookCursor].Name())
	case opMove:
		return fmt.Sprintf("Move %d contact(s) to %s?", n, m.books[m.bookCursor].Name())
	case opDelete:
		return fmt.Sprintf("Permanently delete %d contact(s)?", n)
	}
	return "Continue?"
}
func (m *model) executeOperation() error {
	targets := m.targets()
	switch m.op {
	case opDelete:
		for _, c := range targets {
			if err := m.store.Delete(c); err != nil {
				return err
			}
		}
	case opCopy:
		book := m.books[m.bookCursor]
		for _, c := range targets {
			card := contact.Clone(c.Card)
			delete(card, vcard.FieldUID)
			if _, err := m.store.Save(card, book, ""); err != nil {
				return err
			}
		}
	case opMove:
		book := m.books[m.bookCursor]
		for _, c := range targets {
			if c.Book.Path == book.Path {
				continue
			}
			if _, err := m.store.Save(contact.Clone(c.Card), book, ""); err != nil {
				return err
			}
			if err := m.store.Delete(c); err != nil {
				return err
			}
		}
	}
	return nil
}
func (m *model) reload(err error) {
	if err != nil {
		m.message = "error: " + err.Error()
		return
	}
	cs, e := m.store.Load()
	if e != nil {
		m.message = "error: " + e.Error()
		return
	}
	m.contacts = cs
	m.selected = map[string]bool{}
	m.search.SetValue("")
	m.filter()
	m.message = "saved"
}

func (m *model) startForm(c *contact.Contact, merged []contact.Contact) {
	keys := []string{vcard.FieldFormattedName, vcard.FieldEmail, vcard.FieldTelephone, vcard.FieldOrganization, vcard.FieldTitle, vcard.FieldAddress, vcard.FieldURL, vcard.FieldBirthday, vcard.FieldNote}
	labels := []string{"Full name", "Emails", "Phones", "Organization", "Title", "Address", "URLs", "Birthday", "Notes"}
	values := make([]textinput.Model, len(keys))
	card := vcard.Card{}
	if c != nil {
		card = c.Card
	}
	for i, k := range keys {
		in := textinput.New()
		in.Prompt = ""
		in.CharLimit = 2048
		if k == vcard.FieldEmail || k == vcard.FieldTelephone || k == vcard.FieldURL {
			in.SetValue(strings.Join(card.Values(k), ", "))
		} else {
			in.SetValue(card.Value(k))
		}
		values[i] = in
	}
	book := 0
	if c != nil {
		for i, b := range m.books {
			if b.Path == c.Book.Path {
				book = i
			}
		}
	}
	m.form = formState{fields: keys, labels: labels, values: values, focus: 0, book: book, editing: c, merged: merged}
	m.form.base = contact.Clone(card)
	m.mode = modeForm
	m.focusForm()
}
func (m *model) focusForm() {
	for i := range m.form.values {
		if i == m.form.focus {
			m.form.values[i].Focus()
		} else {
			m.form.values[i].Blur()
		}
	}
}
func (m *model) saveForm() {
	if len(m.books) == 0 {
		m.message = "no addressbooks configured"
		m.mode = modeList
		return
	}
	card := contact.Clone(m.form.base)
	existing := ""
	if m.form.editing != nil {
		existing = m.form.editing.Path
	}
	for i, k := range m.form.fields {
		value := strings.TrimSpace(m.form.values[i].Value())
		delete(card, k)
		if value == "" {
			continue
		}
		if k == vcard.FieldEmail || k == vcard.FieldTelephone || k == vcard.FieldURL {
			for _, v := range strings.Split(value, ",") {
				if v = strings.TrimSpace(v); v != "" {
					card.AddValue(k, v)
				}
			}
		} else {
			card.SetValue(k, value)
		}
	}
	if strings.TrimSpace(card.Value(vcard.FieldFormattedName)) == "" {
		m.message = "full name is required"
		return
	}
	book := m.books[m.form.book]
	if m.form.editing != nil && m.form.editing.Book.Path != book.Path {
		existing = ""
	}
	_, err := m.store.Save(card, book, existing)
	if err == nil && m.form.editing != nil && existing == "" {
		err = m.store.Delete(*m.form.editing)
	}
	if err == nil && len(m.form.merged) > 0 {
		for _, c := range m.form.merged {
			if c.Path != existing {
				if e := m.store.Delete(c); e != nil && !os.IsNotExist(e) {
					err = e
					break
				}
			}
		}
	}
	m.mode = modeList
	m.op = opNone
	m.reload(err)
}

func (m *model) startMerge() {
	targets := m.targets()
	if m.selectedCount() < 2 {
		m.message = "select at least two contacts to merge"
		return
	}
	books := map[string]bool{}
	for _, c := range targets {
		books[c.Book.Path] = true
	}
	m.op = opMerge
	if len(books) > 1 {
		m.mode = modeBooks
		m.bookCursor = 0
		return
	}
	for i, b := range m.books {
		if b.Path == targets[0].Book.Path {
			m.bookCursor = i
		}
	}
	m.prepareMerge(m.books[m.bookCursor])
}
func (m *model) prepareMerge(book config.Source) {
	targets := m.targets()
	cards := make([]vcard.Card, len(targets))
	for i, c := range targets {
		cards[i] = c.Card
	}
	conf := contact.Conflicts(cards)
	m.conflicts = nil
	m.conflictValues = conf
	m.conflictChoice = map[string]string{}
	for k, vals := range conf {
		m.conflicts = append(m.conflicts, k)
		m.conflictChoice[k] = vals[0]
	}
	sortStrings(m.conflicts)
	if len(m.conflicts) > 0 {
		m.mode = modeConflict
		m.conflictCursor = 0
		return
	}
	m.finishMerge()
}
func (m *model) cycleConflict(key string, vals []string, delta int) {
	idx := 0
	for i, v := range vals {
		if v == m.conflictChoice[key] {
			idx = i
		}
	}
	idx = (idx + delta + len(vals)) % len(vals)
	m.conflictChoice[key] = vals[idx]
}
func (m *model) finishMerge() {
	targets := m.targets()
	cards := make([]vcard.Card, len(targets))
	for i, c := range targets {
		cards[i] = c.Card
	}
	merged := contact.Merge(cards, m.conflictChoice)
	book := m.books[m.bookCursor]
	c := contact.Contact{Card: merged, Book: book}
	m.startForm(&c, targets)
	m.form.editing = nil
	m.form.book = m.bookCursor
}

type editorDone struct{ err error }

func (m *model) openEditor() tea.Cmd {
	c := m.current()
	if c == nil {
		return nil
	}
	editor := strings.TrimSpace(m.cfg.Editor)
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], c.Path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDone{err} })
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

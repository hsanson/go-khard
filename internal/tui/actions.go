package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
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
		if m.filterBook != "" && c.Book.Path != m.filterBook {
			continue
		}
		if q == "" || fuzzy(c.SearchText(), q) {
			m.visible = append(m.visible, c)
		}
	}
	m.cursor = 0
	m.offset = 0
}
func (m *model) updateAddressbookFilter(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	total := len(m.books) + 1
	switch k.String() {
	case "esc", "q":
		m.mode = modeList
	case "j", "down":
		m.filterBookCursor = (m.filterBookCursor + 1) % total
	case "k", "up":
		m.filterBookCursor = (m.filterBookCursor - 1 + total) % total
	case "enter":
		m.filterBook = ""
		if m.filterBookCursor > 0 {
			m.filterBook = m.books[m.filterBookCursor-1].Path
		}
		m.selected = map[string]bool{}
		m.filter()
		m.mode = modeList
	}
	return m, nil
}
func (m *model) currentFilterBookCursor() int {
	for i, book := range m.books {
		if book.Path == m.filterBook {
			return i + 1
		}
	}
	return 0
}
func (m *model) filterBookName() string {
	if m.filterBook == "" {
		return "All"
	}
	for _, book := range m.books {
		if book.Path == m.filterBook {
			return book.Name()
		}
	}
	return "All"
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
	if n <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= n {
		return s
	}
	return ansi.Truncate(s, n, "…")
}
func tableCell(s string, width int) string {
	s = clip(s, width)
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
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
	card := make(vcard.Card)
	if c != nil {
		card = contact.Clone(c.Card)
	}
	book := 0
	if c != nil {
		for i, b := range m.books {
			if b.Path == c.Book.Path {
				book = i
			}
		}
	}
	path := ""
	if c != nil {
		path = c.Path
	}
	m.form = formState{card: card, cursor: 1, book: book, editing: c, merged: merged, path: path}
	m.mode = modeForm
}
func (m *model) startShow(c *contact.Contact) {
	if c == nil {
		return
	}
	book := 0
	for i, candidate := range m.books {
		if candidate.Path == c.Book.Path {
			book = i
			break
		}
	}
	m.form = formState{card: contact.Clone(c.Card), cursor: 1, book: book, editing: c, path: c.Path}
	m.mode = modeShow
}
func (m *model) saveForm() {
	if len(m.books) == 0 {
		m.message = "no addressbooks configured"
		m.mode = modeList
		return
	}
	card := contact.Clone(m.form.card)
	existing := ""
	if m.form.editing != nil {
		existing = m.form.editing.Path
	}
	if strings.TrimSpace(card.Value(vcard.FieldFormattedName)) == "" {
		card.SetValue(vcard.FieldFormattedName, formattedNameFromCard(card))
	}
	if strings.TrimSpace(card.Value(vcard.FieldFormattedName)) == "" {
		m.form.errMsg = "Formatted name or a name component is required"
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

func (m *model) startMerge() tea.Cmd {
	targets := m.targets()
	if m.selectedCount() < 2 {
		m.message = "select at least two contacts to merge"
		return nil
	}
	books := map[string]bool{}
	for _, c := range targets {
		books[c.Book.Path] = true
	}
	m.op = opMerge
	if len(books) > 1 {
		m.mode = modeBooks
		m.bookCursor = 0
		return nil
	}
	for i, b := range m.books {
		if b.Path == targets[0].Book.Path {
			m.bookCursor = i
		}
	}
	return m.prepareMerge(m.books[m.bookCursor])
}
func (m *model) prepareMerge(book config.Source) tea.Cmd {
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
		return m.openConflictForm()
	}
	m.finishMerge()
	return nil
}
func (m *model) openConflictForm() tea.Cmd {
	if m.conflictCursor >= len(m.conflicts) {
		m.conflictForm = nil
		m.finishMerge()
		return nil
	}
	key := m.conflicts[m.conflictCursor]
	values := m.conflictValues[key]
	m.conflictValue = values[0]
	options := make([]huh.Option[string], 0, len(values))
	for _, value := range values {
		options = append(options, huh.NewOption(value, value))
	}
	m.conflictForm = popup(huh.NewSelect[string]().
		Title(conflictLabel(key)).
		Description("Select the value to keep").
		Options(options...).
		Value(&m.conflictValue))
	return m.conflictForm.Init()
}
func (m *model) updateActiveConflictForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
		m.conflictForm = nil
		m.mode = modeList
		m.op = opNone
		return m, nil
	}
	updated, cmd := m.conflictForm.Update(msg)
	if form, ok := updated.(*huh.Form); ok {
		m.conflictForm = form
	}
	switch m.conflictForm.State {
	case huh.StateAborted:
		m.conflictForm = nil
		m.mode = modeList
		m.op = opNone
	case huh.StateCompleted:
		key := m.conflicts[m.conflictCursor]
		m.conflictChoice[key] = m.conflictValue
		m.conflictCursor++
		return m, m.openConflictForm()
	}
	return m, cmd
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

func conflictLabel(key string) string {
	labels := map[string]string{
		vcard.FieldFormattedName: "Formatted name",
		vcard.FieldKind:          "Kind",
		"name-prefix":            "Prefix",
		"name-first":             "First name",
		"name-additional":        "Additional name",
		"name-last":              "Last name",
		"name-suffix":            "Suffix",
		vcard.FieldAnniversary:   "Anniversary",
		vcard.FieldBirthday:      "Birthday",
		vcard.FieldNote:          "Note",
	}
	if label := labels[key]; label != "" {
		return label
	}
	return key
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

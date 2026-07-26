package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/config"
	"github.com/hsanson/go-khard/internal/contact"
)

type mode int

const (
	modeList mode = iota
	modeSearch
	modeBooks
	modeConfirm
	modeForm
	modeShow
	modeConflict
	modeCustom
)

type operation int

const (
	opNone operation = iota
	opCopy
	opMove
	opDelete
	opMerge
)

type formState struct {
	fields, labels []string
	values         []textinput.Model
	focus, book    int
	editing        *contact.Contact
	merged         []contact.Contact
	base           vcard.Card
}
type model struct {
	store                         *contact.Store
	cfg                           *config.Config
	contacts, visible             []contact.Contact
	cursor, offset, width, height int
	selected                      map[string]bool
	mode                          mode
	op                            operation
	search                        textinput.Model
	books                         []config.Source
	bookCursor                    int
	form                          formState
	message                       string
	conflicts                     []string
	conflictValues                map[string][]string
	conflictChoice                map[string]string
	conflictCursor                int
}

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
var dim = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
var selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))

func Run(store *contact.Store, cfg *config.Config) error {
	contacts, err := store.Load()
	if err != nil {
		return err
	}
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "name, email, or phone"
	m := &model{store: store, cfg: cfg, contacts: contacts, selected: map[string]bool{}, search: in, books: cfg.Addressbooks(), conflictChoice: map[string]string{}}
	m.filter()
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
func (m *model) Init() tea.Cmd { return nil }
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if done, ok := msg.(editorDone); ok {
		m.reload(done.err)
		return m, nil
	}
	if w, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = w.Width
		m.height = w.Height
		m.clamp()
		return m, nil
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch m.mode {
	case modeSearch:
		return m.updateSearch(k)
	case modeBooks:
		return m.updateBooks(k)
	case modeConfirm:
		return m.updateConfirm(k)
	case modeForm:
		return m.updateForm(k)
	case modeShow:
		if k.String() == "esc" || k.String() == "q" || k.String() == "enter" {
			m.mode = modeList
		}
		return m, nil
	case modeConflict:
		return m.updateConflict(k)
	case modeCustom:
		return m.updateCustom(k)
	}
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor++
	case "k", "up":
		m.cursor--
	case "ctrl+f", "pgdown":
		m.cursor += m.pageSize()
	case "ctrl+b", "pgup":
		m.cursor -= m.pageSize()
	case "/":
		m.mode = modeSearch
		m.search.Focus()
		return m, textinput.Blink
	case " ":
		if c := m.current(); c != nil {
			m.selected[c.Path] = !m.selected[c.Path]
		}
	case "enter":
		if m.current() != nil {
			m.mode = modeShow
		}
	case "a":
		m.startForm(nil, nil)
	case "e":
		if c := m.current(); c != nil {
			m.startForm(c, nil)
		}
	case "ctrl+e":
		return m, m.openEditor()
	case "c":
		m.startBookOperation(opCopy)
	case "x":
		m.startBookOperation(opMove)
	case "d":
		m.startConfirmation(opDelete)
	case "M":
		m.startMerge()
	case "esc":
		m.selected = map[string]bool{}
		m.message = ""
	}
	m.clamp()
	return m, nil
}
func (m *model) updateSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.mode = modeList
		m.search.Blur()
		m.search.SetValue("")
		m.filter()
		return m, nil
	case "enter":
		m.mode = modeList
		m.search.Blur()
		m.filter()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(k)
	m.filter()
	return m, cmd
}
func (m *model) updateBooks(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q":
		m.mode = modeList
		m.op = opNone
	case "j", "down":
		m.bookCursor++
	case "k", "up":
		m.bookCursor--
	case "enter":
		if len(m.books) > 0 {
			if m.op == opMerge {
				m.prepareMerge(m.books[m.bookCursor])
			} else {
				m.mode = modeConfirm
			}
		}
	}
	if m.bookCursor < 0 {
		m.bookCursor = 0
	}
	if m.bookCursor >= len(m.books) {
		m.bookCursor = len(m.books) - 1
	}
	return m, nil
}
func (m *model) updateConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "y", "Y", "enter":
		err := m.executeOperation()
		m.mode = modeList
		m.op = opNone
		m.reload(err)
	case "n", "N", "esc", "q":
		m.mode = modeList
		m.op = opNone
	}
	return m, nil
}
func (m *model) updateForm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "esc" {
		m.mode = modeList
		return m, nil
	}
	if k.String() == "ctrl+s" {
		m.saveForm()
		return m, nil
	}
	if k.String() == "tab" || k.String() == "down" || k.String() == "ctrl+n" {
		m.form.focus = (m.form.focus + 1) % (len(m.form.values) + 1)
		m.focusForm()
		return m, nil
	}
	if k.String() == "shift+tab" || k.String() == "up" || k.String() == "ctrl+p" {
		m.form.focus = (m.form.focus + len(m.form.values)) % (len(m.form.values) + 1)
		m.focusForm()
		return m, nil
	}
	if m.form.focus == len(m.form.values) {
		switch k.String() {
		case "j", "right":
			m.form.book = (m.form.book + 1) % max(1, len(m.books))
		case "k", "left":
			m.form.book = (m.form.book - 1 + max(1, len(m.books))) % max(1, len(m.books))
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.form.values[m.form.focus], cmd = m.form.values[m.form.focus].Update(k)
	return m, cmd
}
func (m *model) updateConflict(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.conflicts) == 0 {
		m.finishMerge()
		return m, nil
	}
	key := m.conflicts[m.conflictCursor]
	vals := m.conflictValues[key]
	switch k.String() {
	case "esc", "q":
		m.mode = modeList
		m.op = opNone
	case "j", "down":
		m.conflictCursor = (m.conflictCursor + 1) % len(m.conflicts)
	case "k", "up":
		m.conflictCursor = (m.conflictCursor - 1 + len(m.conflicts)) % len(m.conflicts)
	case "h", "left":
		m.cycleConflict(key, vals, -1)
	case "l", "right", " ":
		m.cycleConflict(key, vals, 1)
	case "e":
		m.search.SetValue(m.conflictChoice[key])
		m.search.Prompt = "custom " + key + ": "
		m.search.Focus()
		m.mode = modeCustom
		return m, textinput.Blink
	case "enter":
		m.finishMerge()
	}
	return m, nil
}
func (m *model) updateCustom(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "esc" {
		m.mode = modeConflict
		m.search.Blur()
		return m, nil
	}
	if k.String() == "enter" {
		m.conflictChoice[m.conflicts[m.conflictCursor]] = m.search.Value()
		m.mode = modeConflict
		m.search.Blur()
		m.search.Prompt = "/ "
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(k)
	return m, cmd
}
func (m *model) View() string {
	if m.mode == modeShow {
		return m.showView()
	}
	if m.mode == modeForm {
		return m.formView()
	}
	if m.mode == modeBooks {
		return m.bookView()
	}
	if m.mode == modeConfirm {
		return m.confirmView()
	}
	if m.mode == modeConflict {
		return m.conflictView()
	}
	if m.mode == modeCustom {
		return "\n  " + accent.Render("Enter a custom conflict value") + "\n\n  " + m.search.View() + "\n\n  enter accept · esc cancel"
	}
	return m.listView()
}
func (m *model) listView() string {
	var b strings.Builder
	b.WriteString(accent.Render(" go-khard — Contacts ") + "\n")
	if m.mode == modeSearch {
		b.WriteString(" " + m.search.View() + "\n")
	} else {
		b.WriteString(dim.Render(" / search   space select   enter show   a add   e edit   ctrl-e editor   c copy   x move   d delete   M merge   q quit") + "\n")
	}
	nameW, bookW := max(16, (m.width*30)/100), max(10, (m.width*16)/100)
	emailW := max(18, (m.width*28)/100)
	phoneW := max(12, m.width-nameW-bookW-emailW-9)
	b.WriteString(dim.Render(fmt.Sprintf("   %-*s %-*s %-*s %-*s", nameW, "NAME", bookW, "ADDRESSBOOK", emailW, "EMAIL", phoneW, "PHONE")) + "\n")
	end := min(len(m.visible), m.offset+m.pageSize())
	for i := m.offset; i < end; i++ {
		c := m.visible[i]
		mark := " "
		if m.selected[c.Path] {
			mark = "✓"
		}
		prefix := " "
		if i == m.cursor {
			prefix = "›"
		}
		line := fmt.Sprintf("%s%s %-*s %-*s %-*s %-*s", prefix, mark, nameW, clip(c.Name(), nameW), bookW, clip(c.Book.Name(), bookW), emailW, clip(strings.Join(c.Emails(), ", "), emailW), phoneW, clip(strings.Join(c.Phones(), ", "), phoneW))
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + dim.Render(fmt.Sprintf(" %d contacts · %d selected", len(m.visible), m.selectedCount())))
	if m.message != "" {
		b.WriteString(" · " + m.message)
	}
	return b.String()
}
func (m *model) showView() string {
	c := m.current()
	if c == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(accent.Render(" Contact details ") + "\n\n")
	keys := make([]string, 0, len(c.Card))
	for k := range c.Card {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, f := range c.Card[k] {
			b.WriteString(fmt.Sprintf("  %-14s %s\n", k, f.Value))
		}
	}
	b.WriteString("\n" + dim.Render(" enter/esc/q back"))
	return b.String()
}
func (m *model) bookView() string {
	var b strings.Builder
	b.WriteString(accent.Render(" Select target addressbook ") + "\n\n")
	for i, x := range m.books {
		p := "  "
		if i == m.bookCursor {
			p = "› "
		}
		line := p + x.Name() + "  " + dim.Render(x.Path)
		if i == m.bookCursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + dim.Render(" j/k move · enter choose · esc cancel"))
	return b.String()
}
func (m *model) confirmView() string {
	return "\n  " + accent.Render("Confirm") + "\n\n  " + m.confirmText() + "\n\n  y/enter confirm · n/esc cancel"
}
func (m *model) conflictView() string {
	var b strings.Builder
	b.WriteString(accent.Render(" Resolve merge conflicts ") + "\n\n")
	for i, k := range m.conflicts {
		p := "  "
		if i == m.conflictCursor {
			p = "› "
		}
		line := fmt.Sprintf("%s%-12s %s", p, k, m.conflictChoice[k])
		if i == m.conflictCursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + dim.Render(" j/k field · h/l choose · e custom value · enter review merge · esc cancel"))
	return b.String()
}
func (m *model) formView() string {
	var b strings.Builder
	title := "Add contact"
	if m.form.editing != nil {
		title = "Edit contact"
	}
	if len(m.form.merged) > 0 {
		title = "Review merged contact"
	}
	b.WriteString(accent.Render(" "+title+" ") + "\n\n")
	for i, v := range m.form.values {
		p := "  "
		if i == m.form.focus {
			p = "› "
		}
		b.WriteString(fmt.Sprintf("%s%-14s %s\n", p, m.form.labels[i], v.View()))
	}
	book := "(no addressbooks)"
	if len(m.books) > 0 {
		book = m.books[m.form.book].Name()
	}
	p := "  "
	if m.form.focus == len(m.form.values) {
		p = "› "
	}
	b.WriteString(fmt.Sprintf("%s%-14s %s\n", p, "Addressbook", book))
	b.WriteString("\n" + dim.Render(" tab/shift-tab fields · ctrl+s save · esc cancel"))
	return b.String()
}

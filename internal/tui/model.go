package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
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
	modeAddressbookFilter
)

type operation int

const (
	opNone operation = iota
	opCopy
	opMove
	opDelete
	opMerge
)

type editorRow struct {
	key, label, value string
	index             int
	section, add      bool
}

type formState struct {
	card       vcard.Card
	cursor     int
	offset     int
	book       int
	editing    *contact.Contact
	merged     []contact.Contact
	path       string
	activeForm *huh.Form
	activeRow  editorRow
	tmp        []string
	tmpTypes   []string
	errMsg     string
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
	filterBook                    string
	filterBookCursor              int
	form                          formState
	message                       string
	conflicts                     []string
	conflictValues                map[string][]string
	conflictChoice                map[string]string
	conflictCursor                int
	conflictForm                  *huh.Form
	conflictValue                 string
}

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
var dim = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
var fieldNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
var fieldValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
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
	if w, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = w.Width
		m.height = w.Height
		m.clamp()
		return m, nil
	}
	if m.mode == modeForm && m.form.activeForm != nil {
		return m.updateActiveEditorForm(msg)
	}
	if m.mode == modeConflict && m.conflictForm != nil {
		return m.updateActiveConflictForm(msg)
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
	case modeAddressbookFilter:
		return m.updateAddressbookFilter(k)
	case modeForm:
		return m.updateForm(k)
	case modeShow:
		switch k.String() {
		case "esc", "q", "enter":
			m.mode = modeList
		case "j", "down":
			m.moveEditorCursor(m.editorRows(), 1)
		case "k", "up":
			m.moveEditorCursor(m.editorRows(), -1)
		case "e":
			if m.form.editing != nil {
				m.startForm(m.form.editing, nil)
			}
		}
		return m, nil
	case modeConflict:
		return m, nil
	case modeCustom:
		return m.updateCustom(k)
	}
	switch k.String() {
	case "q", "esc", "ctrl+c":
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
		if c := m.current(); c != nil {
			m.startShow(c)
		}
	case "a":
		m.startForm(nil, nil)
	case "b":
		m.mode = modeAddressbookFilter
		m.filterBookCursor = m.currentFilterBookCursor()
	case "e":
		if c := m.current(); c != nil {
			m.startForm(c, nil)
		}
	case "c":
		m.startBookOperation(opCopy)
	case "x":
		m.startBookOperation(opMove)
	case "ctrl+d":
		m.startConfirmation(opDelete)
	case "M":
		return m, m.startMerge()
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
	var cmd tea.Cmd
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
			switch m.op {
			case opMerge:
				cmd = m.prepareMerge(m.books[m.bookCursor])
			default:
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
	return m, cmd
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
	rows := m.editorRows()
	switch k.String() {
	case "j", "down", "tab":
		m.moveEditorCursor(rows, 1)
	case "k", "up", "shift+tab":
		m.moveEditorCursor(rows, -1)
	case "enter":
		return m, m.openEditorPopup(rows)
	case "ctrl+d":
		m.deleteEditorRow(rows)
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
		return m.formView()
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
	if m.mode == modeAddressbookFilter {
		return m.addressbookFilterView()
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
		b.WriteString(dim.Render(" / search   b addressbook   space select   enter show   a add   e edit   c copy   x move   ctrl-d delete   M merge   q quit") + "\n")
	}
	nameW, bookW := max(16, (m.width*30)/100), max(10, (m.width*16)/100)
	emailW := max(18, (m.width*28)/100)
	phoneW := max(12, m.width-nameW-bookW-emailW-9)
	header := "   " + tableCell("NAME", nameW) + " " + tableCell("ADDRESSBOOK", bookW) + " " + tableCell("EMAIL", emailW) + " " + tableCell("PHONE", phoneW)
	b.WriteString(dim.Render(header) + "\n")
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
		line := prefix + mark + " " +
			tableCell(c.Name(), nameW) + " " +
			tableCell(c.Book.Name(), bookW) + " " +
			tableCell(c.PreferredEmail(), emailW) + " " +
			tableCell(c.PreferredPhone(), phoneW)
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + dim.Render(fmt.Sprintf(" %d contacts · %d selected", len(m.visible), m.selectedCount())))
	b.WriteString(" · " + dim.Render("addressbook: "+m.filterBookName()))
	if m.message != "" {
		b.WriteString(" · " + m.message)
	}
	return b.String()
}
func (m *model) addressbookFilterView() string {
	var b strings.Builder
	b.WriteString(accent.Render(" Filter by addressbook ") + "\n\n")
	options := append([]config.Source{{DisplayName: "All"}}, m.books...)
	for i, book := range options {
		prefix := "  "
		if i == m.filterBookCursor {
			prefix = "› "
		}
		label := book.Name()
		if i == 0 {
			label = "All"
		}
		line := prefix + label
		if i == m.filterBookCursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + dim.Render(" j/k move · enter apply · esc cancel"))
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
	if m.conflictForm == nil {
		return ""
	}
	progress := fmt.Sprintf(" Conflict %d of %d ", m.conflictCursor+1, len(m.conflicts))
	header := accent.Render(" Resolve merge conflicts ") + "\n" + dim.Render(progress+"· j/k choose · enter accept · esc cancel")
	popupWidth := min(64, max(30, m.width-8))
	popup := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 2).Width(popupWidth).Render(m.conflictForm.View())
	return header + "\n" + lipgloss.Place(m.width, max(8, m.height-2), lipgloss.Center, lipgloss.Center, popup)
}
func (m *model) formView() string {
	var b strings.Builder
	title := "Add contact"
	if m.mode == modeShow {
		title = "Contact details"
	} else if m.form.editing != nil {
		title = "Edit contact"
	}
	if len(m.form.merged) > 0 {
		title = "Review merged contact"
	}
	b.WriteString(accent.Render(" "+title+" ") + "\n")
	if m.mode == modeShow {
		b.WriteString(dim.Render(" j/k navigate · e edit · enter/esc/q back") + "\n")
	} else {
		b.WriteString(dim.Render(" j/k navigate · enter edit · ctrl+d remove entry · ctrl+s save · esc cancel") + "\n")
	}
	rows := m.editorRows()
	page := max(5, m.height-5)
	if m.form.cursor < m.form.offset {
		m.form.offset = m.form.cursor
	}
	if m.form.cursor >= m.form.offset+page {
		m.form.offset = m.form.cursor - page + 1
	}
	for i := m.form.offset; i < min(len(rows), m.form.offset+page); i++ {
		row := rows[i]
		if row.section {
			if i > m.form.offset {
				b.WriteString("\n")
			}
			label := " " + row.label + " "
			ruleWidth := max(0, m.width-lipgloss.Width(label)-2)
			separator := lipgloss.NewStyle().Width(max(10, m.width)).Foreground(lipgloss.Color("244")).Bold(true).Render(label + strings.Repeat("─", ruleWidth))
			b.WriteString(separator + "\n")
			continue
		}
		prefix := "  "
		if i == m.form.cursor {
			prefix = "› "
		}
		label := fieldNameStyle.Width(20).Render(row.label)
		valueWidth := max(12, m.width-27)
		valueLines := strings.Split(row.value, "\n")
		line := fmt.Sprintf("%s%s: %s", prefix, label, fieldValueStyle.Render(clip(valueLines[0], valueWidth)))
		if len(valueLines) > 1 {
			indent := strings.Repeat(" ", lipgloss.Width(prefix)+20+2)
			for _, valueLine := range valueLines[1:] {
				line += "\n" + indent + fieldValueStyle.Render(clip(valueLine, valueWidth))
			}
		}
		if row.add {
			line = prefix + accent.Render(row.label)
		}
		if i == m.form.cursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if m.form.errMsg != "" {
		b.WriteString("\n " + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.form.errMsg))
	}
	base := b.String()
	if m.form.activeForm == nil {
		return base
	}
	popupWidth := min(64, max(30, m.width-8))
	popup := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 2).Width(popupWidth).Render(m.form.activeForm.View())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
}

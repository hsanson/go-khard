package tui

import (
	"fmt"
	"net/mail"
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
	modeConflict
	modeCustom
	modeAddressbookFilter
	modeEmailMatches
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
	messageErr                    bool
	showHelp                      bool
	conflicts                     []string
	conflictValues                map[string][]string
	conflictChoice                map[string]string
	conflictCursor                int
	conflictForm                  *huh.Form
	conflictValue                 string
	emailSender                   contact.Contact
	emailMatches                  []contact.Contact
	emailMatchCursor              int
	mergeTargets                  []contact.Contact
	quitAfterSave                 bool
}

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
var dim = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
var fieldNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
var fieldValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
var selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
var errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

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

func RunAddEmail(store *contact.Store, cfg *config.Config, sender *mail.Address) error {
	contacts, err := store.Load()
	if err != nil {
		return err
	}
	if len(cfg.Addressbooks()) == 0 {
		return fmt.Errorf("no addressbooks configured")
	}
	card := make(vcard.Card)
	name := strings.TrimSpace(sender.Name)
	if name == "" {
		name = sender.Address
	}
	card.SetValue(vcard.FieldFormattedName, name)
	card.AddValue(vcard.FieldEmail, sender.Address)
	in := textinput.New()
	m := &model{
		store: store, cfg: cfg, contacts: contacts, visible: contacts,
		selected: map[string]bool{}, search: in, books: cfg.Addressbooks(),
		conflictChoice: map[string]string{}, quitAfterSave: true,
		emailSender:  contact.Contact{Card: card},
		emailMatches: contact.SimilarContacts(contacts, sender.Name, sender.Address),
	}
	if len(m.emailMatches) == 0 {
		m.startForm(&m.emailSender, nil)
		m.form.editing = nil
		m.form.path = ""
	} else {
		m.mode = modeEmailMatches
	}
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
	if m.showHelp {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "?", "esc", "q":
				m.showHelp = false
			}
		}
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
	if k.String() == "?" && m.mode != modeCustom {
		m.showHelp = true
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
	case modeEmailMatches:
		return m.updateEmailMatches(k)
	case modeForm:
		return m.updateForm(k)
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
			m.startForm(c, nil)
		}
	case "n":
		m.startForm(nil, nil)
	case "b":
		m.mode = modeAddressbookFilter
		m.filterBookCursor = m.currentFilterBookCursor()
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
	if k.String() == "esc" || k.String() == "q" || k.String() == "ctrl+c" {
		if m.quitAfterSave {
			return m, tea.Quit
		}
		m.mode = modeList
		return m, nil
	}
	if k.String() == "ctrl+s" {
		return m, m.saveForm()
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
	if m.showHelp {
		return m.renderHelpOverlay(max(58, m.width*2/3), max(16, m.height*2/3))
	}
	var content string
	switch m.mode {
	case modeForm:
		content = m.formView()
	case modeBooks:
		content = m.bookView()
	case modeConfirm:
		content = m.confirmView()
	case modeAddressbookFilter:
		content = m.addressbookFilterView()
	case modeEmailMatches:
		content = m.emailMatchesView()
	case modeConflict:
		content = m.conflictView()
	case modeCustom:
		content = "\n  " + accent.Render("Enter a custom conflict value") + "\n\n  " + m.search.View()
	default:
		content = m.listView()
	}
	if legend := m.shortcutsLegend(); legend != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", dim.Render(" "+legend))
	}
	return content
}

func (m *model) emailMatchesView() string {
	var b strings.Builder
	b.WriteString(accent.Render(" Similar contacts ") + "\n")
	b.WriteString(dim.Render("Choose a contact to merge with the email sender, or create a new contact.") + "\n\n")
	for i, candidate := range m.emailMatches {
		line := "  " + candidate.Name() + "  " + dim.Render(candidate.PreferredEmail()+" · "+candidate.Book.Name())
		if i == m.emailMatchCursor {
			line = selectedStyle.Render("› " + strings.TrimPrefix(line, "  "))
		}
		b.WriteString(line + "\n")
	}
	createIndex := len(m.emailMatches)
	line := "  Create new"
	if m.emailMatchCursor == createIndex {
		line = selectedStyle.Render("› Create new")
	}
	b.WriteString(line + "\n")
	return b.String()
}
func (m *model) listView() string {
	var b strings.Builder
	b.WriteString(m.listHeader() + "\n")
	if m.message != "" {
		message := dim.Render(m.message)
		if m.messageErr {
			message = errorStyle.Render(m.message)
		}
		b.WriteString(" " + message + "\n")
	}
	if m.mode == modeSearch {
		b.WriteString(" " + m.search.View() + "\n")
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
	return b.String()
}

func (m *model) listHeader() string {
	title := accent.Render(" go-khard — Contacts ")
	stats := dim.Render(fmt.Sprintf("%d contacts · %d selected · addressbook: %s ", len(m.visible), m.selectedCount(), m.filterBookName()))
	gap := max(1, m.width-lipgloss.Width(title)-lipgloss.Width(stats))
	return title + strings.Repeat(" ", gap) + stats
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
	return b.String()
}
func (m *model) confirmView() string {
	return "\n  " + accent.Render("Confirm") + "\n\n  " + m.confirmText()
}
func (m *model) conflictView() string {
	if m.conflictForm == nil {
		return ""
	}
	progress := fmt.Sprintf(" Conflict %d of %d ", m.conflictCursor+1, len(m.conflicts))
	header := accent.Render(" Resolve merge conflicts ") + "\n" + dim.Render(progress)
	popupWidth := min(64, max(30, m.width-8))
	popup := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 2).Width(popupWidth).Render(editorPopupView(m.conflictForm, popupWidth-4))
	return header + "\n" + lipgloss.Place(m.width, max(8, m.height-2), lipgloss.Center, lipgloss.Center, popup)
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
	b.WriteString(accent.Render(" "+title+" ") + "\n")
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
		b.WriteString("\n " + errorStyle.Render(m.form.errMsg))
	}
	base := b.String()
	if m.form.activeForm == nil {
		return base
	}
	popupWidth := min(64, max(30, m.width-8))
	popup := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 2).Width(popupWidth).Render(editorPopupView(m.form.activeForm, popupWidth-4))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
}

func (m *model) shortcutsLegend() string {
	if (m.mode == modeForm && m.form.activeForm != nil) || (m.mode == modeConflict && m.conflictForm != nil) {
		return ""
	}
	switch m.mode {
	case modeSearch:
		return "[esc] Cancel  [enter] Apply  [?] Help"
	case modeForm:
		legend := "[esc/q] Cancel  [j/k] Next / Prev  [enter] Edit"
		if row, ok := m.currentEditorRow(); ok && removableEditorRow(row) {
			legend += "  [ctrl+d] Remove"
		}
		return legend + "  [ctrl+s] Save  [?] Help"
	case modeBooks:
		return "[esc/q] Cancel  [j/k] Next / Prev  [enter] Choose  [?] Help"
	case modeConfirm:
		return "[esc/q/n] Cancel  [enter/y] Confirm  [?] Help"
	case modeAddressbookFilter:
		return "[esc/q] Cancel  [j/k] Next / Prev  [enter] Apply  [?] Help"
	case modeEmailMatches:
		return "[esc/q] Cancel  [j/k] Next / Prev  [enter] Choose  [?] Help"
	case modeCustom:
		return "[esc] Cancel  [enter] Accept"
	case modeConflict:
		return "[esc/q] Cancel  [?] Help"
	default:
		return "[esc/q] Exit  [j/k] Next / Prev  [/] Search  [enter] Open  [n] New  [?] Help"
	}
}

func (m *model) helpLines() []string {
	switch m.mode {
	case modeSearch:
		return []string{
			"Type        Search names, email, and phone",
			"←/→         Move within search text",
			"enter       Apply search",
			"esc         Cancel search",
			"?           Toggle help",
		}
	case modeForm:
		lines := []string{
			"esc, q      Cancel contact editor",
			"ctrl+c      Cancel contact editor",
			"j/k         Next / previous editable item",
			"↑/↓         Next / previous editable item",
			"tab         Next editable item",
			"shift+tab   Previous editable item",
			"enter       Edit selected field",
		}
		if row, ok := m.currentEditorRow(); ok && removableEditorRow(row) {
			lines = append(lines, "ctrl+d      Remove selected entry")
		}
		return append(lines,
			"ctrl+s      Save contact",
			"?           Toggle help",
		)
	case modeBooks:
		return []string{
			"esc, q      Cancel",
			"j/k         Next / previous addressbook",
			"↑/↓         Next / previous addressbook",
			"enter       Choose addressbook",
			"?           Toggle help",
		}
	case modeConfirm:
		return []string{
			"enter, y    Confirm operation",
			"esc, q, n   Cancel operation",
			"?           Toggle help",
		}
	case modeAddressbookFilter:
		return []string{
			"esc, q      Cancel",
			"j/k         Next / previous addressbook",
			"↑/↓         Next / previous addressbook",
			"enter       Apply filter",
			"?           Toggle help",
		}
	case modeEmailMatches:
		return []string{
			"esc, q      Cancel",
			"ctrl+c      Cancel",
			"j/k         Next / previous match",
			"↑/↓         Next / previous match",
			"enter       Choose match",
			"?           Toggle help",
		}
	case modeCustom:
		return []string{"enter       Accept value", "esc         Cancel"}
	default:
		return []string{
			"esc, q      Exit",
			"ctrl+c      Exit",
			"j/k         Next / previous contact",
			"↑/↓         Next / previous contact",
			"ctrl+f/b    Page down / page up",
			"/           Search contacts",
			"enter       Open contact editor",
			"n           New contact",
			"space       Select/unselect contact",
			"b           Filter by addressbook",
			"c           Copy selected contacts",
			"x           Move selected contacts",
			"ctrl+d      Delete selected contacts",
			"M           Merge selected contacts",
			"?           Toggle help",
		}
	}
}

func (m *model) renderHelpOverlay(width, height int) string {
	width = max(58, width)
	lines := m.helpLines()
	height = max(height, len(lines)+4)
	title := accent.Render("Shortcuts")
	body := lipgloss.NewStyle().Width(width - 4).Render(strings.Join(lines, "\n"))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245")).
		Padding(1, 2).
		Width(width).
		Height(height).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body))
	if m.width <= 0 || m.height <= 0 {
		return box
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func editorPopupView(form *huh.Form, width int) string {
	if form == nil {
		return ""
	}
	view := form.WithWidth(max(24, width)).WithShowHelp(true).WithShowErrors(true).View()
	return lipgloss.JoinVertical(lipgloss.Left, view, "", dim.Render("[esc] Cancel"))
}

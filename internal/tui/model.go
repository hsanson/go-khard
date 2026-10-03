package tui

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

type dialogFocus int

const (
	dialogFocusControl dialogFocus = iota
	dialogFocusPrimary
	dialogFocusCancel
	dialogFocusDelete
)

type dialogAction struct {
	label       string
	focus       dialogFocus
	destructive bool
}

type editorRow struct {
	key, label, value string
	index             int
	section, add      bool
}

type formState struct {
	card                                        vcard.Card
	cursor, offset, book                        int
	editing                                     *contact.Contact
	merged                                      []contact.Contact
	path                                        string
	activeForm                                  *huh.Form
	datePicker                                  *contactDatePicker
	activeRow                                   editorRow
	tmp, tmpTypes                               []string
	errMsg                                      string
	dialogFocus                                 dialogFocus
	dialogField, dialogFields, bookBeforeDialog int
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
	dialogFocus                   dialogFocus
	mouse                         *mouseState
	styles                        Styles
	formTheme                     *huh.Theme
	themeMonitor                  *themeMonitor
}

func Run(store *contact.Store, cfg *config.Config) error {
	contacts, err := store.Load()
	if err != nil {
		return err
	}
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "name, email, or phone"
	m := &model{store: store, cfg: cfg, contacts: contacts, selected: map[string]bool{}, search: in, books: cfg.Addressbooks(), conflictChoice: map[string]string{}, mouse: &mouseState{}}
	m.initTheme()
	defer m.themeMonitor.close()
	m.filter()
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
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
		mouse:        &mouseState{},
	}
	m.initTheme()
	defer m.themeMonitor.close()
	if len(m.emailMatches) == 0 {
		m.startForm(&m.emailSender, nil)
		m.form.editing = nil
		m.form.path = ""
	} else {
		m.mode = modeEmailMatches
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func (m *model) initTheme() {
	var theme themeStyles
	m.themeMonitor, theme = newThemeMonitor()
	m.applyTheme(theme)
}

func (m *model) applyTheme(theme themeStyles) {
	m.styles = theme.styles
	if m.formTheme == nil {
		m.formTheme = theme.formTheme
	} else {
		*m.formTheme = *theme.formTheme
	}
	m.search.PromptStyle = m.styles.Accent
	m.search.TextStyle = m.styles.FieldValue
	m.search.PlaceholderStyle = m.styles.Dim
	m.search.CompletionStyle = m.styles.Dim
	m.search.Cursor.Style = m.styles.Accent
}

func (m *model) ensureTheme() {
	if m.formTheme == nil {
		m.applyTheme(defaultInteractiveTheme())
	}
}

func (m *model) Init() tea.Cmd { return m.themeMonitor.wait() }
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case themeStylesMsg:
		m.applyTheme(msg.theme)
		return m, m.themeMonitor.wait()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clamp()
		return m, nil
	case tea.MouseMsg:
		return m.updateMouse(tea.MouseEvent(msg))
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
	if m.mode == modeForm && m.form.datePicker != nil {
		return m.updateActiveDatePicker(msg)
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
	if k.String() == "?" {
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
	case modeEmailMatches:
		return m.updateEmailMatches(k)
	case modeForm:
		return m.updateForm(k)
	case modeConflict:
		return m, nil
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
	case "tab":
		m.cycleAddressbook(1)
	case "shift+tab":
		m.cycleAddressbook(-1)
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
	case "c":
		m.startBookOperation(opCopy)
	case "m":
		if m.selectedCount() > 0 {
			m.startBookOperation(opMove)
		}
	case "d":
		if m.selectedCount() > 0 {
			m.startConfirmation(opDelete)
		}
	case "M":
		if m.selectedCount() > 1 {
			return m, m.startMerge()
		}
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
		return m, nil
	case "down", "ctrl+j":
		m.cursor++
		m.clamp()
		return m, nil
	case "up", "ctrl+k":
		m.cursor--
		m.clamp()
		return m, nil
	case "tab":
		m.cycleAddressbook(1)
		return m, nil
	case "shift+tab":
		m.cycleAddressbook(-1)
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
		m.cancelBookOperation()
	case "tab":
		cycleDialogAction(&m.dialogFocus, 1, false)
	case "shift+tab":
		cycleDialogAction(&m.dialogFocus, -1, false)
	case "j", "down":
		moveDialogCursor(&m.dialogFocus, &m.bookCursor, len(m.books), 1, false)
	case "k", "up":
		moveDialogCursor(&m.dialogFocus, &m.bookCursor, len(m.books), -1, false)
	case "left", "h":
		selectDialogAction(&m.dialogFocus, dialogFocusPrimary)
	case "right", "l":
		selectDialogAction(&m.dialogFocus, dialogFocusCancel)
	case "enter", " ":
		return m.activateBookDialog()
	}
	return m, nil
}

func (m *model) updateConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q", "n", "N":
		m.cancelConfirmation()
	case "tab", "right", "l", "down", "j", "shift+tab", "left", "h", "up", "k":
		if m.dialogFocus == dialogFocusPrimary {
			m.dialogFocus = dialogFocusCancel
		} else {
			m.dialogFocus = dialogFocusPrimary
		}
	case "y", "Y":
		m.dialogFocus = dialogFocusPrimary
		return m.activateConfirmation()
	case "enter", " ":
		return m.activateConfirmation()
	}
	return m, nil
}

func (m *model) updateForm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "esc" || k.String() == "q" || k.String() == "ctrl+c" {
		return m.cancelForm()
	}
	if k.String() == "ctrl+s" {
		return m.saveOrConfirmForm()
	}
	rows := m.editorRows()
	switch k.String() {
	case "j", "down", "tab":
		m.moveEditorCursor(rows, 1)
	case "k", "up", "shift+tab":
		m.moveEditorCursor(rows, -1)
	case "enter":
		if row, ok := m.currentEditorRow(); ok {
			switch row.key {
			case "form-save":
				return m.saveOrConfirmForm()
			case "form-cancel":
				return m.cancelForm()
			}
		}
		return m, m.openEditorPopup(rows)
	}
	return m, nil
}
func (m *model) mainHeight() int {
	if m.height <= 0 {
		return 30
	}
	footerHeight := lipgloss.Height(m.footerView())
	if footerHeight == 0 {
		return m.height
	}
	return max(1, m.height-footerHeight-1)
}

func (m *model) footerView() string {
	legend := m.shortcutsLegend()
	if legend == "" {
		return ""
	}
	if m.width > 1 {
		legend = ansi.Wordwrap(legend, m.width-1, "")
	}
	return m.styles.Dim.Render(" " + legend)
}

func (m *model) View() string {
	m.ensureTheme()
	if m.mouse == nil {
		m.mouse = &mouseState{}
	}
	m.mouse.reset()
	mainHeight := m.mainHeight()
	var content string
	if m.showHelp {
		content = m.renderHelpOverlay(mainHeight)
	} else {
		switch m.mode {
		case modeForm:
			content = m.formView()
		case modeBooks:
			content = m.bookView()
		case modeConfirm:
			content = m.confirmView()
		case modeEmailMatches:
			content = m.emailMatchesView()
		case modeConflict:
			content = m.conflictView()
		default:
			content = m.listView()
		}
	}
	if m.height > 0 {
		content = lipgloss.NewStyle().Height(mainHeight).MaxHeight(mainHeight).Render(content)
	}
	if footer := m.footerView(); footer != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", footer)
	}
	surface := m.styles.Surface
	if m.width > 0 {
		surface = surface.Width(m.width)
	}
	if m.height > 0 {
		surface = surface.Height(m.height)
	}
	return surface.Render(content)
}

func (m *model) emailMatchesView() string {
	var b strings.Builder
	optionHits := make([]mouseHit, 0, len(m.emailMatches)+1)
	b.WriteString(m.styles.Accent.Render("Similar contacts") + "\n")
	b.WriteString(m.styles.Dim.Render("Choose a contact to merge with the email sender, or create a new contact.") + "\n\n")
	for i, candidate := range m.emailMatches {
		line := "  " + candidate.Name() + "  " + m.styles.Dim.Render(candidate.PreferredEmail()+" · "+candidate.Book.Name())
		if i == m.emailMatchCursor && m.dialogFocus == dialogFocusControl {
			line = m.styles.Selected.Render("› " + strings.TrimPrefix(line, "  "))
		}
		optionHits = append(optionHits, mouseHit{rect: mouseRect{x: 0, y: 3 + i, width: max(1, ansi.StringWidth(line)), height: 1}, kind: mouseEmailMatch, index: i})
		b.WriteString(line + "\n")
	}
	createIndex := len(m.emailMatches)
	line := "  Create new"
	if m.emailMatchCursor == createIndex && m.dialogFocus == dialogFocusControl {
		line = m.styles.Selected.Render("› Create new")
	}
	optionHits = append(optionHits, mouseHit{rect: mouseRect{x: 0, y: 3 + createIndex, width: max(1, ansi.StringWidth(line)), height: 1}, kind: mouseEmailMatch, index: createIndex})
	b.WriteString(line)
	content, actionHits := dialogWithActions(b.String(), []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}, m.dialogFocus, m.styles)
	return m.centerDialog(content, append(optionHits, actionHits...))
}

func (m *model) listView() string {
	var b strings.Builder
	y := 0
	b.WriteString(m.listHeader() + "\n")
	y++
	if m.message != "" {
		message := m.styles.Dim.Render(m.message)
		if m.messageErr {
			message = m.styles.Error.Render(m.message)
		}
		b.WriteString(" " + message + "\n")
		y++
	}
	if m.mode == modeSearch {
		b.WriteString(" " + m.search.View() + "\n")
		y++
	}
	nameW, bookW := max(16, (m.width*30)/100), max(10, (m.width*16)/100)
	emailW := max(18, (m.width*28)/100)
	phoneW := max(12, m.width-nameW-bookW-emailW-9)
	header := "   " + tableCell("NAME", nameW) + " " + tableCell("ADDRESSBOOK", bookW) + " " + tableCell("EMAIL", emailW) + " " + tableCell("PHONE", phoneW)
	b.WriteString(m.styles.Dim.Render(header) + "\n")
	y++
	end := min(len(m.visible), m.offset+m.pageSize())
	for i := m.offset; i < end; i++ {
		contact := m.visible[i]
		mark := " "
		if m.selected[contact.Path] {
			mark = "✓"
		}
		prefix := " "
		if i == m.cursor {
			prefix = "›"
		}
		line := prefix + mark + " " +
			tableCell(contact.Name(), nameW) + " " +
			tableCell(contact.Book.Name(), bookW) + " " +
			tableCell(contact.PreferredEmail(), emailW) + " " +
			tableCell(contact.PreferredPhone(), phoneW)
		if i == m.cursor {
			line = m.styles.Selected.Render(line)
		}
		m.addMouseHit(mouseHit{rect: mouseRect{x: 0, y: y, width: max(1, m.width), height: 1}, kind: mouseContact, index: i})
		b.WriteString(line + "\n")
		y++
	}
	if len(m.visible) == 0 {
		b.WriteString(m.styles.Dim.Render("   No matches") + "\n")
	}
	return b.String()
}

func (m *model) listHeader() string {
	title := m.styles.Accent.Render(" go-khard — Contacts ")
	statsText := fmt.Sprintf(" %d   %d   %s ", len(m.visible), m.selectedCount(), m.filterBookName())
	stats := m.styles.Dim.Render(statsText)
	gap := max(1, m.width-lipgloss.Width(title)-lipgloss.Width(stats))
	statsX := lipgloss.Width(title) + gap
	bookPrefix := fmt.Sprintf(" %d   %d   ", len(m.visible), m.selectedCount())
	m.addMouseHit(mouseHit{
		rect: mouseRect{x: statsX + ansi.StringWidth(bookPrefix), y: 0, width: max(1, ansi.StringWidth(m.filterBookName())), height: 1},
		kind: mouseAddressbook,
	})
	return title + strings.Repeat(" ", gap) + stats
}
func renderButton(action dialogAction, selected bool, styles Styles) string {
	style := styles.Button
	if action.focus == dialogFocusPrimary {
		style = styles.PrimaryButton
	}
	if action.destructive {
		style = styles.DestructiveButton
	}
	label := action.label
	if selected {
		style = styles.FocusedButton
		if action.destructive {
			style = styles.FocusedDestructiveButton
			label = "›" + label + "‹"
		}
	}
	return style.Render(label)
}

func dialogWithActions(content string, actions []dialogAction, focus dialogFocus, styles Styles) (string, []mouseHit) {
	var row strings.Builder
	hits := make([]mouseHit, 0, len(actions))
	x := 0
	y := lipgloss.Height(content) + 1
	for i, action := range actions {
		button := renderButton(action, focus == action.focus, styles)
		width := lipgloss.Width(button)
		switch {
		case action.focus == dialogFocusDelete:
			gap := width + 4
			row.WriteString(strings.Repeat(" ", gap))
			x += gap
		case i > 0:
			row.WriteString("  ")
			x += 2
		}
		hits = append(hits, mouseHit{rect: mouseRect{x: x, y: y, width: width, height: 1}, kind: mouseDialogAction, focus: action.focus})
		row.WriteString(button)
		x += width
	}
	return lipgloss.JoinVertical(lipgloss.Left, content, "", row.String()), hits
}

func (m *model) addOffsetMouseHits(hits []mouseHit, x, y int) {
	for _, hit := range hits {
		hit.rect.x += x
		hit.rect.y += y
		m.addMouseHit(hit)
	}
}

func dialogOuterWidth(width int) int {
	if width <= 0 {
		width = 80
	}
	return min(width, max(7, width*3/5))
}

func dialogStyleWidth(width int) int {
	return max(1, dialogOuterWidth(width)-2)
}

func dialogBodyWidth(width int) int {
	return max(1, dialogOuterWidth(width)-6)
}

func (m *model) centerDialog(content string, hits []mouseHit) string {
	width, height := max(1, m.width), m.mainHeight()
	outerWidth := dialogOuterWidth(m.width)
	box := m.styles.Dialog.
		Width(dialogStyleWidth(m.width)).
		MaxWidth(outerWidth).
		Render(content)
	x := max(0, (width-lipgloss.Width(box))/2)
	y := max(0, (height-lipgloss.Height(box))/2)
	m.addOffsetMouseHits(hits, x+3, y+2)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *model) bookView() string {
	var b strings.Builder
	hits := make([]mouseHit, 0, len(m.books)+2)
	b.WriteString(m.styles.Accent.Render("Select target addressbook") + "\n\n")
	for i, book := range m.books {
		prefix := "  "
		if i == m.bookCursor && m.dialogFocus == dialogFocusControl {
			prefix = "› "
		}
		line := prefix + book.Name() + "  " + m.styles.Dim.Render(book.Path)
		if i == m.bookCursor && m.dialogFocus == dialogFocusControl {
			line = m.styles.Selected.Render(line)
		}
		hits = append(hits, mouseHit{rect: mouseRect{x: 0, y: 2 + i, width: max(1, ansi.StringWidth(line)), height: 1}, kind: mouseBook, index: i})
		b.WriteString(line + "\n")
	}
	content, actionHits := dialogWithActions(strings.TrimSuffix(b.String(), "\n"), []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}, m.dialogFocus, m.styles)
	return m.centerDialog(content, append(hits, actionHits...))
}

func (m *model) confirmView() string {
	destructive := m.op == opDelete || m.op == opMerge
	content := lipgloss.JoinVertical(lipgloss.Left, m.styles.Accent.Render("Confirm"), "", m.confirmText())
	content, hits := dialogWithActions(content, []dialogAction{
		{label: "Confirm", focus: dialogFocusPrimary, destructive: destructive},
		{label: "Cancel", focus: dialogFocusCancel},
	}, m.dialogFocus, m.styles)
	return m.centerDialog(content, hits)
}

func (m *model) conflictView() string {
	if m.conflictForm == nil {
		return ""
	}
	progress := m.styles.Dim.Render(fmt.Sprintf("Conflict %d of %d", m.conflictCursor+1, len(m.conflicts)))
	form := editorPopupView(m.conflictForm, dialogBodyWidth(m.width))
	content := lipgloss.JoinVertical(lipgloss.Left, m.styles.Accent.Render("Resolve merge conflicts"), progress, "", form)
	content, hits := dialogWithActions(content, []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}, m.dialogFocus, m.styles)
	return m.centerDialog(content, hits)
}

type editorLineHit struct {
	row, x, width int
}

func (m *model) renderEditorRow(row editorRow, selected bool, width int) string {
	prefix := "  "
	if selected {
		prefix = "› "
	}
	if row.add {
		action := dialogAction{label: row.label, focus: dialogFocusPrimary}
		line := prefix + renderButton(action, selected, m.styles)
		return lipgloss.NewStyle().Width(width).Render(line)
	}
	label := m.styles.FieldName.Width(20).Render(row.label)
	valueWidth := max(1, width-27)
	valueLines := strings.Split(row.value, "\n")
	line := fmt.Sprintf("%s%s: %s", prefix, label, m.styles.FieldValue.Render(clip(valueLines[0], valueWidth)))
	if len(valueLines) > 1 {
		indent := strings.Repeat(" ", lipgloss.Width(prefix)+22)
		for _, valueLine := range valueLines[1:] {
			line += "\n" + indent + m.styles.FieldValue.Render(clip(valueLine, valueWidth))
		}
	}
	if selected {
		line = m.styles.Selected.Render(line)
	}
	return line
}

func renderFormActions(saveSelected, cancelSelected bool, styles Styles) (string, int, int, int) {
	save := renderButton(dialogAction{label: "Save", focus: dialogFocusPrimary}, saveSelected, styles)
	cancel := renderButton(dialogAction{label: "Cancel", focus: dialogFocusCancel}, cancelSelected, styles)
	saveWidth := lipgloss.Width(save)
	cancelX := saveWidth + 2
	return save + "  " + cancel, saveWidth, cancelX, lipgloss.Width(cancel)
}

func (m *model) formBaseView() string {
	title := "Add contact"
	if m.form.editing != nil {
		title = "Edit contact"
	}
	if len(m.form.merged) > 0 {
		title = "Review merged contact"
	}
	header := []string{m.styles.Accent.Render(" " + title + " ")}
	if m.form.errMsg != "" {
		header = append(header, m.styles.Error.Render(" "+m.form.errMsg))
	}
	bodyHeight := max(1, m.mainHeight()-len(header))
	rows := m.editorRows()
	lines := make([]string, 0, len(rows))
	lineHits := make([][]editorLineHit, 0, len(rows))
	selectedLine := 0
	for i := 0; i < len(rows); i++ {
		row := rows[i]
		if row.section {
			if len(lines) > 0 {
				lines = append(lines, "")
				lineHits = append(lineHits, nil)
			}
			label := " " + row.label + " "
			ruleWidth := max(0, m.width-lipgloss.Width(label)-2)
			line := m.styles.Section.Width(max(10, m.width)).Render(label + strings.Repeat("─", ruleWidth))
			lines = append(lines, line)
			lineHits = append(lineHits, nil)
			continue
		}
		if row.key == "form-save" && i+1 < len(rows) && rows[i+1].key == "form-cancel" {
			if i == m.form.cursor || i+1 == m.form.cursor {
				selectedLine = len(lines)
			}
			line, saveWidth, cancelX, cancelWidth := renderFormActions(i == m.form.cursor, i+1 == m.form.cursor, m.styles)
			lines = append(lines, line)
			lineHits = append(lineHits, []editorLineHit{{row: i, width: saveWidth}, {row: i + 1, x: cancelX, width: cancelWidth}})
			i++
			continue
		}
		if i == m.form.cursor {
			selectedLine = len(lines)
		}
		rendered := strings.Split(m.renderEditorRow(row, i == m.form.cursor, max(10, m.width)), "\n")
		for _, line := range rendered {
			lines = append(lines, line)
			if selectableEditorRow(row) {
				lineHits = append(lineHits, []editorLineHit{{row: i, width: max(1, m.width)}})
			} else {
				lineHits = append(lineHits, nil)
			}
		}
	}
	start := 0
	if len(lines) > bodyHeight && selectedLine >= bodyHeight {
		start = selectedLine - bodyHeight + 1
	}
	end := min(len(lines), start+bodyHeight)
	m.form.offset = start
	for y, hits := range lineHits[start:end] {
		for _, hit := range hits {
			m.addMouseHit(mouseHit{rect: mouseRect{x: hit.x, y: len(header) + y, width: hit.width, height: 1}, kind: mouseEditorRow, index: hit.row})
		}
	}
	body := lipgloss.NewStyle().Width(max(1, m.width)).Height(bodyHeight).MaxHeight(bodyHeight).Render(strings.Join(lines[start:end], "\n"))
	return lipgloss.JoinVertical(lipgloss.Left, append(header, body)...)
}

func (m *model) formView() string {
	base := m.formBaseView()
	if m.form.activeForm == nil && m.form.datePicker == nil {
		return base
	}
	popupWidth := dialogBodyWidth(m.width)
	var content string
	var hits []mouseHit
	if m.form.datePicker != nil {
		picker, pickerHits := m.form.datePicker.render(m.styles)
		for i := range pickerHits {
			pickerHits[i].rect.y += 2
		}
		content = lipgloss.JoinVertical(lipgloss.Left, m.styles.Accent.Render(m.form.activeRow.label), "", picker)
		content = lipgloss.NewStyle().Width(popupWidth).MaxWidth(popupWidth).Render(content)
		hits = pickerHits
	} else {
		content = editorPopupView(m.form.activeForm, popupWidth)
	}
	actions := []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}
	if m.editorDialogHasDelete() {
		actions = append(actions, dialogAction{label: "Delete", focus: dialogFocusDelete, destructive: true})
	}
	content, actionHits := dialogWithActions(content, actions, m.form.dialogFocus, m.styles)
	return m.overlayDialog(base, content, append(hits, actionHits...))
}

func (m *model) subduedSurface(base string, width, height int) []string {
	source := strings.Split(base, "\n")
	lines := make([]string, height)
	for y := range lines {
		line := ""
		if y < len(source) {
			line = ansi.Strip(source[y])
		}
		line = ansi.Cut(line, 0, width)
		line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
		lines[y] = m.styles.Dim.Render(line)
	}
	return lines
}

func (m *model) overlayDialog(base, content string, hits []mouseHit) string {
	width, height := max(1, m.width), m.mainHeight()
	outerWidth := dialogOuterWidth(m.width)
	box := m.styles.Dialog.
		Width(dialogStyleWidth(m.width)).
		MaxWidth(outerWidth).
		Render(content)
	boxLines := strings.Split(box, "\n")
	boxWidth := lipgloss.Width(box)
	x := max(0, (width-boxWidth)/2)
	y := max(0, (height-len(boxLines))/2)
	lines := m.subduedSurface(base, width, height)
	for i, boxLine := range boxLines {
		if y+i >= len(lines) {
			break
		}
		boxLine += strings.Repeat(" ", max(0, boxWidth-ansi.StringWidth(boxLine)))
		lines[y+i] = ansi.Cut(lines[y+i], 0, x) + boxLine + ansi.Cut(lines[y+i], min(width, x+boxWidth), width)
	}
	m.addOffsetMouseHits(hits, x+3, y+2)
	return strings.Join(lines, "\n")
}

func (m *model) shortcutsLegend() string {
	if m.showHelp {
		return "[?/esc/q] Close"
	}
	switch m.mode {
	case modeSearch:
		return "[type] Search  [↑/↓ ctrl-j/ctrl-k] Move  [tab/shift+tab] Addressbook  [enter] Keep  [esc] Clear  [?] Help"
	case modeForm:
		if m.form.datePicker != nil {
			return "[h/l ←/→] Day  [j/k ↑/↓] Week  [ctrl+k/j] Month  [ctrl+h/l] Year  [[/]] Month  [t] Today  [space] Clear  [tab/shift+tab] Actions  [enter] Apply  [esc/q] Cancel"
		}
		if m.form.activeForm != nil {
			if m.form.activeRow.key == vcard.FieldNote {
				return "[ctrl+enter] New line  [↑/↓] Cursor  [tab/shift+tab] Move  [enter] Apply  [esc] Cancel"
			}
			switch m.form.activeForm.GetFocusedField().(type) {
			case *huh.MultiSelect[string]:
				return "[j/k h/l ←/→] Choice  [space] Toggle  [tab/↓ shift+tab/↑] Move  [enter] Next  [esc] Cancel"
			case *huh.Select[string], *huh.Select[int]:
				return "[j/k h/l ←/→] Change  [tab/↓ shift+tab/↑] Actions  [enter] Apply  [esc] Cancel"
			default:
				return "[tab/↓ shift+tab/↑] Move  [enter] Apply  [esc] Cancel"
			}
		}
		return "[esc/q] Cancel  [j/k ↑/↓ tab/shift+tab] Move  [enter] Edit  [ctrl+s] Save  [?] Help"
	case modeBooks:
		return "[j/k ↑/↓] Move  [tab/shift+tab] Actions  [enter/space] Apply  [esc/q] Cancel  [?] Help"
	case modeConfirm:
		return "[tab/←/→] Choose  [enter/y] Confirm  [esc/q/n] Cancel  [?] Help"
	case modeEmailMatches:
		return "[j/k ↑/↓] Move  [tab/shift+tab] Actions  [enter/space] Apply  [esc/q] Cancel  [?] Help"
	case modeConflict:
		return "[←/→] Change  [tab/↑/↓] Actions  [enter/space] Apply  [esc/q] Cancel  [?] Help"
	default:
		legend := "[esc] quit  [tab] addressbook  [/] search  [enter] open  [n] new  [spc] select  [?] help"
		if selected := m.selectedCount(); selected > 0 {
			legend += "  [m] move  [d] Delete"
			if selected > 1 {
				legend += "  [M] Merge"
			}
		}
		return legend
	}
}

func (m *model) helpLines() []string {
	switch m.mode {
	case modeSearch:
		return []string{
			"Type          Search names, email, and phone",
			"ctrl-j/k, ↑/↓ Move through filtered contacts",
			"tab           Next addressbook",
			"shift+tab     Previous addressbook",
			"enter         Keep search",
			"esc           Clear search",
		}
	case modeForm:
		return []string{
			"esc, q        Cancel contact editor",
			"ctrl+c        Cancel contact editor",
			"j/k, ↑/↓      Next / previous editable item",
			"tab           Next editable item",
			"shift+tab     Previous editable item",
			"enter         Edit field or activate button",
			"ctrl+s        Save contact",
		}
	case modeBooks:
		return []string{
			"j/k, ↑/↓      Move through addressbooks and actions",
			"tab           Focus actions",
			"enter         Apply focused action",
			"esc, q        Cancel",
		}
	case modeConfirm:
		return []string{
			"tab, ←/→      Choose Confirm / Cancel",
			"enter, y      Activate confirmation",
			"esc, q, n     Cancel operation",
		}
	case modeEmailMatches:
		return []string{
			"j/k, ↑/↓      Move through matches and actions",
			"tab           Focus actions",
			"enter         Apply focused action",
			"esc, q        Cancel",
		}
	case modeConflict:
		return []string{
			"←/→           Change conflict value",
			"tab, ↑/↓      Move to actions",
			"enter         Apply focused action",
			"esc, q        Cancel merge",
		}
	default:
		lines := []string{
			"esc, q        Exit",
			"ctrl+c        Exit",
			"j/k, ↑/↓      Next / previous contact",
			"ctrl+f/b      Page down / page up",
			"tab           Next addressbook",
			"shift+tab     Previous addressbook",
			"/             Search contacts",
			"enter         Open contact editor",
			"n             New contact",
			"space         Select/unselect contact",
			"c             Copy current/selected contacts",
		}
		if m.selectedCount() > 0 {
			lines = append(lines, "m             Move selected contacts")
			if m.selectedCount() > 1 {
				lines = append(lines, "M             Merge selected contacts")
			}
			lines = append(lines, "d             Delete selected contacts")
		}
		return lines
	}
}

func (m *model) renderHelpOverlay(height int) string {
	width := dialogBodyWidth(m.width)
	content := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.Accent.Render("Shortcuts"),
		"",
		lipgloss.NewStyle().Width(width).MaxWidth(width).Render(strings.Join(m.helpLines(), "\n")),
	)
	outerWidth := dialogOuterWidth(m.width)
	box := m.styles.HelpDialog.
		Width(dialogStyleWidth(m.width)).
		MaxWidth(outerWidth).
		Render(content)
	if m.width <= 0 || height <= 0 {
		return box
	}
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func editorPopupView(form *huh.Form, width int) string {
	if form == nil {
		return ""
	}
	return form.WithWidth(max(1, width)).WithShowHelp(false).WithShowErrors(true).View()
}

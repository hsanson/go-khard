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

type workspacePane uint8

const (
	paneContacts workspacePane = iota
	paneDetails
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
	formOrigin                    workspacePane
	focusedPane                   workspacePane
	detailScroll                  int
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
	deleteTarget                  *contact.Contact
	quitAfterSave                 bool
	dialogFocus                   dialogFocus
	mouse                         *mouseState
	styles                        Styles
	formTheme                     *huh.Theme
	themeMonitor                  *themeMonitor
	detailStatus                  detailActionStatus
	detailActionStatusID          uint64
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
	m.search.TextStyle = m.styles.Value
	m.search.PlaceholderStyle = m.styles.Subtle
	m.search.CompletionStyle = m.styles.Subtle
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
	case detailActionResultMsg:
		if msg.id != m.detailActionStatusID {
			return m, nil
		}
		m.detailStatus.done = true
		m.detailStatus.failed = msg.err != nil
		if msg.err != nil {
			return m, nil
		}
		if m.detailStatus.action != detailActionCopy {
			m.detailStatus = detailActionStatus{}
			return m, nil
		}
		return m, expireDetailAction(msg.id)
	case detailActionExpiredMsg:
		if msg.id == m.detailActionStatusID {
			m.detailStatus = detailActionStatus{}
		}
		return m, nil
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
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if k.String() == "tab" || k.String() == "shift+tab" {
		m.moveWorkspaceFocus()
		return m, nil
	}
	if m.focusedPane == paneDetails {
		switch k.String() {
		case "q":
			return m, tea.Quit
		case "esc", "h", "left":
			m.focusedPane = paneContacts
		case "j", "down":
			m.detailScroll++
		case "k", "up":
			m.detailScroll = max(0, m.detailScroll-1)
		case "enter", "e":
			if c := m.current(); c != nil {
				m.startForm(c, nil)
			}
		case "n":
			m.startForm(nil, nil)
		}
		return m, nil
	}

	before := m.currentPath()
	switch k.String() {
	case "q", "esc":
		return m, tea.Quit
	case "j", "down":
		m.cursor++
	case "k", "up":
		m.cursor--
	case "ctrl+f", "pgdown":
		m.cursor += m.pageSize()
	case "ctrl+b", "pgup":
		m.cursor -= m.pageSize()
	case "[":
		m.cycleAddressbook(-1)
	case "]":
		m.cycleAddressbook(1)
	case "/":
		m.mode = modeSearch
		m.search.Focus()
		return m, textinput.Blink
	case " ":
		if c := m.current(); c != nil {
			m.selected[c.Path] = !m.selected[c.Path]
		}
	case "enter", "l", "right":
		if m.current() != nil {
			m.focusedPane = paneDetails
			m.detailScroll = 0
		}
	case "e":
		if c := m.current(); c != nil {
			m.startForm(c, nil)
		}
	case "n":
		m.startForm(nil, nil)
	case "c":
		if m.canCopy() {
			m.startBookOperation(opCopy)
		}
	case "m":
		if m.canMove() {
			m.startBookOperation(opMove)
		}
	case "d":
		if m.selectedCount() > 0 {
			m.startConfirmation(opDelete)
		}
	case "M":
		if m.canMerge() {
			return m, m.startMerge()
		}
	}
	m.clamp()
	m.resetDetailsIfContactChanged(before)
	return m, nil
}
func (m *model) updateSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	before := m.currentPath()
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
		m.resetDetailsIfContactChanged(before)
		return m, nil
	case "up", "ctrl+k":
		m.cursor--
		m.clamp()
		m.resetDetailsIfContactChanged(before)
		return m, nil
	case "[":
		m.cycleAddressbook(-1)
		return m, nil
	case "]":
		m.cycleAddressbook(1)
		return m, nil
	case "tab", "shift+tab":
		m.mode = modeList
		m.search.Blur()
		m.moveWorkspaceFocus()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(k)
	m.filter()
	return m, cmd
}
func (m *model) updateBooks(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q", "ctrl+c":
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
	case "esc", "q", "ctrl+c", "n", "N":
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
	case "h", "left":
		m.moveFormAction(rows, -1)
	case "l", "right":
		m.moveFormAction(rows, 1)
	case "enter", " ":
		if row, ok := m.currentEditorRow(); ok {
			switch row.key {
			case "form-save":
				return m.saveOrConfirmForm()
			case "form-cancel":
				return m.cancelForm()
			case "form-delete":
				m.startFormDelete()
				return m, nil
			}
		}
		if k.String() == "enter" {
			return m, m.openEditorPopup(rows)
		}
	}
	return m, nil
}

func (m *model) moveFormAction(rows []editorRow, delta int) {
	if m.form.cursor < 0 || m.form.cursor >= len(rows) || !strings.HasPrefix(rows[m.form.cursor].key, "form-") {
		return
	}
	actions := make([]int, 0, 3)
	current := 0
	for i, row := range rows {
		if strings.HasPrefix(row.key, "form-") {
			if i == m.form.cursor {
				current = len(actions)
			}
			actions = append(actions, i)
		}
	}
	if len(actions) > 0 {
		m.form.cursor = actions[(current+delta+len(actions))%len(actions)]
	}
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
	return m.styles.Subtle.Render(" " + legend)
}

func (m *model) View() string {
	m.ensureTheme()
	if m.mouse == nil {
		m.mouse = &mouseState{}
	}
	m.mouse.reset()
	mainHeight := m.mainHeight()
	var content string
	switch {
	case m.showHelp:
		content = m.renderHelpOverlay(mainHeight)
	case m.quitAfterSave:
		content = m.fullScreenView()
	default:
		content = m.workspaceView(mainHeight)
	}
	if m.height > 0 {
		content = lipgloss.NewStyle().Height(mainHeight).MaxHeight(mainHeight).Render(content)
	}
	if footer := m.footerView(); footer != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", footer)
	}
	style := lipgloss.NewStyle()
	if m.width > 0 {
		style = style.Width(m.width)
	}
	if m.height > 0 {
		style = style.Height(m.height)
	}
	return style.Render(content)
}

func (m *model) fullScreenView() string {
	switch m.mode {
	case modeForm:
		return m.formView()
	case modeBooks:
		return m.bookView()
	case modeConfirm:
		return m.confirmView()
	case modeEmailMatches:
		return m.emailMatchesView()
	case modeConflict:
		return m.conflictView()
	}
	return ""
}

func (m *model) emailMatchesView() string {
	var b strings.Builder
	optionHits := make([]mouseHit, 0, len(m.emailMatches)+1)
	b.WriteString(m.styles.Accent.Render("Similar contacts") + "\n")
	b.WriteString(m.styles.Subtle.Render("Choose a contact to merge with the email sender, or create a new contact.") + "\n\n")
	for i, candidate := range m.emailMatches {
		line := "  " + candidate.Name() + "  " + m.styles.Subtle.Render(candidate.PreferredEmail()+" · "+candidate.Book.Name())
		if i == m.emailMatchCursor && m.dialogFocus == dialogFocusControl {
			line = m.styles.SelectedRow.Render("› " + strings.TrimPrefix(line, "  "))
		}
		optionHits = append(optionHits, mouseHit{rect: mouseRect{x: 0, y: 3 + i, width: max(1, ansi.StringWidth(line)), height: 1}, kind: mouseEmailMatch, index: i})
		b.WriteString(line + "\n")
	}
	createIndex := len(m.emailMatches)
	line := "  Create new"
	if m.emailMatchCursor == createIndex && m.dialogFocus == dialogFocusControl {
		line = m.styles.SelectedRow.Render("› Create new")
	}
	optionHits = append(optionHits, mouseHit{rect: mouseRect{x: 0, y: 3 + createIndex, width: max(1, ansi.StringWidth(line)), height: 1}, kind: mouseEmailMatch, index: createIndex})
	b.WriteString(line)
	content, actionHits := dialogWithActions(b.String(), []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}, m.dialogFocus, dialogBodyWidth(m.width), m.styles)
	return m.centerDialog(content, append(optionHits, actionHits...))
}

func renderButton(action dialogAction, selected bool, styles Styles) string {
	style := styles.SecondaryButton
	if action.focus == dialogFocusPrimary {
		style = styles.PrimaryButton
	}
	if action.destructive {
		style = styles.DangerButton
	}
	if selected {
		style = styles.FocusedButton
		if action.destructive {
			style = styles.FocusedDanger
		}
	}
	return style.Render(action.label)
}

func renderActionRows(actions []dialogAction, focus dialogFocus, width int, styles Styles) (string, []mouseHit) {
	width = max(1, width)
	var row strings.Builder
	rows := make([]string, 0, 2)
	hits := make([]mouseHit, 0, len(actions))
	rowWidth, y := 0, 0
	for _, action := range actions {
		button := renderButton(action, focus == action.focus, styles)
		buttonWidth := lipgloss.Width(button)
		x := rowWidth
		if action.focus == dialogFocusDelete {
			minX := rowWidth
			if rowWidth > 0 {
				minX += 2
			}
			if rowWidth > 0 && minX+buttonWidth > width {
				rows = append(rows, row.String())
				row.Reset()
				rowWidth = 0
				y++
				x = max(0, width-buttonWidth)
			} else {
				x = max(minX, width-buttonWidth)
			}
		} else if rowWidth > 0 {
			x += 2
		}
		row.WriteString(strings.Repeat(" ", max(0, x-rowWidth)))
		hits = append(hits, mouseHit{
			rect:  mouseRect{x: x, y: y, width: buttonWidth, height: 1},
			kind:  mouseDialogAction,
			focus: action.focus,
		})
		row.WriteString(button)
		rowWidth = x + buttonWidth
	}
	rows = append(rows, row.String())
	return strings.Join(rows, "\n"), hits
}

func joinDialogActions(content, row string, hits []mouseHit) (string, []mouseHit) {
	y := lipgloss.Height(content) + 1
	for i := range hits {
		hits[i].rect.y += y
	}
	return lipgloss.JoinVertical(lipgloss.Left, content, "", row), hits
}

func dialogWithActions(content string, actions []dialogAction, focus dialogFocus, width int, styles Styles) (string, []mouseHit) {
	row, hits := renderActionRows(actions, focus, width, styles)
	return joinDialogActions(content, row, hits)
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
	box := m.styles.Overlay.
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
		line := prefix + book.Name() + "  " + m.styles.Subtle.Render(book.Path)
		if i == m.bookCursor && m.dialogFocus == dialogFocusControl {
			line = m.styles.SelectedRow.Render(line)
		}
		hits = append(hits, mouseHit{rect: mouseRect{x: 0, y: 2 + i, width: max(1, ansi.StringWidth(line)), height: 1}, kind: mouseBook, index: i})
		b.WriteString(line + "\n")
	}
	content, actionHits := dialogWithActions(strings.TrimSuffix(b.String(), "\n"), []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}, m.dialogFocus, dialogBodyWidth(m.width), m.styles)
	return m.centerDialog(content, append(hits, actionHits...))
}

func (m *model) confirmView() string {
	destructive := m.op == opDelete || m.op == opMerge
	content := lipgloss.JoinVertical(lipgloss.Left, m.styles.Accent.Render("Confirm"), "", m.confirmText())
	content, hits := dialogWithActions(content, []dialogAction{
		{label: "Confirm", focus: dialogFocusPrimary, destructive: destructive},
		{label: "Cancel", focus: dialogFocusCancel},
	}, m.dialogFocus, dialogBodyWidth(m.width), m.styles)
	return m.centerDialog(content, hits)
}

func (m *model) conflictView() string {
	if m.conflictForm == nil {
		return ""
	}
	progress := m.styles.Subtle.Render(fmt.Sprintf("Conflict %d of %d", m.conflictCursor+1, len(m.conflicts)))
	form := m.conflictForm.WithWidth(dialogBodyWidth(m.width)).WithShowHelp(false).WithShowErrors(true).View()
	content := lipgloss.JoinVertical(lipgloss.Left, m.styles.Accent.Render("Resolve merge conflicts"), progress, "", form)
	content, hits := dialogWithActions(content, []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}, m.dialogFocus, dialogBodyWidth(m.width), m.styles)
	return m.centerDialog(content, hits)
}

type editorLineHit struct {
	row, line, x, width int
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
	label := m.styles.Accent.Width(20).Render(row.label)
	valueWidth := max(1, width-27)
	valueLines := strings.Split(row.value, "\n")
	line := fmt.Sprintf("%s%s: %s", prefix, label, m.styles.Value.Render(clip(valueLines[0], valueWidth)))
	if len(valueLines) > 1 {
		indent := strings.Repeat(" ", lipgloss.Width(prefix)+22)
		for _, valueLine := range valueLines[1:] {
			line += "\n" + indent + m.styles.Value.Render(clip(valueLine, valueWidth))
		}
	}
	if selected {
		line = m.styles.SelectedRow.Render(line)
	}
	return line
}

func renderFormActions(saveSelected, cancelSelected, deleteSelected, showDelete bool, width int, styles Styles) (string, []editorLineHit) {
	focus := dialogFocusControl
	switch {
	case saveSelected:
		focus = dialogFocusPrimary
	case cancelSelected:
		focus = dialogFocusCancel
	case deleteSelected:
		focus = dialogFocusDelete
	}
	actions := []dialogAction{
		{label: "Save", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}
	if showDelete {
		actions = append(actions, dialogAction{label: "Delete", focus: dialogFocusDelete, destructive: true})
	}
	rendered, hits := renderActionRows(actions, focus, width, styles)
	lineHits := make([]editorLineHit, 0, len(hits))
	for _, hit := range hits {
		row := 0
		switch hit.focus {
		case dialogFocusCancel:
			row = 1
		case dialogFocusDelete:
			row = 2
		}
		lineHits = append(lineHits, editorLineHit{row: row, line: hit.rect.y, x: hit.rect.x, width: hit.rect.width})
	}
	return rendered, lineHits
}

func (m *model) formBaseView() string {
	title := "Add contact"
	if m.form.editing != nil {
		title = "Edit contact"
	}
	if len(m.form.merged) > 0 {
		title = "Review merged contact"
	}
	header := []string{m.styles.PanelTitle.Render(title)}
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
			line := m.styles.Disabled.Bold(true).Width(max(10, m.width)).Render(label + strings.Repeat("─", ruleWidth))
			lines = append(lines, line)
			lineHits = append(lineHits, nil)
			continue
		}
		if row.key == "form-save" && i+1 < len(rows) && rows[i+1].key == "form-cancel" {
			showDelete := i+2 < len(rows) && rows[i+2].key == "form-delete"
			deleteSelected := showDelete && i+2 == m.form.cursor
			line, hits := renderFormActions(i == m.form.cursor, i+1 == m.form.cursor, deleteSelected, showDelete, max(1, m.width), m.styles)
			actionLines := strings.Split(line, "\n")
			actionHits := make([][]editorLineHit, len(actionLines))
			for index := range hits {
				hits[index].row += i
				if hits[index].row == m.form.cursor {
					selectedLine = len(lines) + hits[index].line
				}
				actionHits[hits[index].line] = append(actionHits[hits[index].line], hits[index])
			}
			lines = append(lines, actionLines...)
			lineHits = append(lineHits, actionHits...)
			if showDelete {
				i += 2
			} else {
				i++
			}
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
	actions := []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
	}
	if m.form.datePicker != nil {
		actions = append(actions, dialogAction{label: "Clear", focus: dialogFocusDelete, destructive: true})
	} else if m.editorDialogHasDelete() {
		actions = append(actions, dialogAction{label: "Delete", focus: dialogFocusDelete, destructive: true})
	}
	actionRows, actionHits := renderActionRows(actions, m.form.dialogFocus, popupWidth, m.styles)

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
		overlayFrameHeight := lipgloss.Height(m.styles.Overlay.Render(""))
		formHeight := max(1, m.mainHeight()-overlayFrameHeight-1-lipgloss.Height(actionRows))
		content = editorPopupView(m.form.activeForm, popupWidth, formHeight)
	}
	content, actionHits = joinDialogActions(content, actionRows, actionHits)
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
		lines[y] = m.styles.Subtle.Render(line)
	}
	return lines
}

func (m *model) overlayDialog(base, content string, hits []mouseHit) string {
	width, height := max(1, m.width), m.mainHeight()
	outerWidth := dialogOuterWidth(m.width)
	box := m.styles.Overlay.
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
		return "[type] Search  [↑/↓ ctrl-j/ctrl-k] Move  [[/]] Addressbook  [tab/shift+tab] Pane  [enter] Keep  [esc] Clear  [?] Help"
	case modeForm:
		if m.form.datePicker != nil {
			return "[h/l ←/→] Day/actions  [j/k ↑/↓] Week  [ctrl+k/j] Month  [ctrl+h/l] Year  [[/]] Month  [tab/shift+tab] Actions  [enter/space] Apply/Clear  [esc/q] Cancel"
		}
		if m.form.activeForm != nil {
			if m.form.activeRow.key == vcard.FieldNote {
				return "[ctrl+enter] New line  [↑/↓] Cursor  [tab/shift+tab] Move  [enter] Apply  [esc] Cancel"
			}
			switch m.form.activeForm.GetFocusedField().(type) {
			case *huh.MultiSelect[string]:
				return "[j/k h/l ←/→] Choice  [space] Toggle  [tab/↓ shift+tab/↑] Move  [enter] Next  [esc] Cancel"
			case *huh.Select[string], *huh.Select[int]:
				return "[j/k h/l ←/→] Change  [tab/↓ shift+tab/↑] Move  [enter] Next/Apply  [esc] Cancel"
			default:
				return "[tab/↓ shift+tab/↑] Move  [enter] Next/Apply  [esc] Cancel"
			}
		}
		return "[esc/q] Cancel  [j/k ↑/↓ tab/shift+tab] Move  [h/l ←/→] Actions  [enter] Edit/apply  [ctrl+s] Save  [?] Help"
	case modeBooks:
		return "[j/k ↑/↓] Move  [tab/shift+tab] Actions  [enter/space] Apply  [esc/q/ctrl+c] Cancel  [?] Help"
	case modeConfirm:
		return "[tab/←/→] Choose  [enter/y] Confirm  [esc/q/n/ctrl+c] Cancel  [?] Help"
	case modeEmailMatches:
		return "[j/k ↑/↓] Move  [tab/shift+tab] Actions  [enter/space] Apply  [esc/q/ctrl+c] Cancel  [?] Help"
	case modeConflict:
		return "[←/→] Change  [tab/↑/↓] Actions  [enter/space] Apply  [esc/q/ctrl+c] Cancel  [?] Help"
	}
	if m.focusedPane == paneDetails {
		return "[q] Exit  [h/esc] Contacts  [j/k ↑/↓] Scroll  [enter/e] Edit  [n] New  [tab/shift+tab] Pane  [?] Help"
	}
	legend := "[esc/q] Exit  [j/k ↑/↓] Move  [tab/shift+tab l] Details  [[/]] Addressbook  [/] Search  [e] Edit  [n] New  [space] Select  [?] Help"
	if m.canCopy() {
		legend += "  [c] Copy"
	}
	if m.canMove() {
		legend += "  [m] Move"
	}
	if m.canMerge() {
		legend += "  [M] Merge"
	}
	if m.selectedCount() > 0 {
		legend += "  [d] Delete"
	}
	return legend
}

func (m *model) helpLines() []string {
	switch m.mode {
	case modeSearch:
		return []string{
			"Type              Search names, email, and phone",
			"ctrl-j/k, ↑/↓     Move through filtered contacts",
			"[ / ]             Previous / next addressbook",
			"tab / shift-tab   Change pane",
			"enter             Keep search",
			"esc               Clear search",
		}
	case modeForm:
		return []string{
			"esc, q, ctrl+c    Cancel contact editor",
			"j/k, ↑/↓          Next / previous editable item",
			"tab / shift-tab   Next / previous editable item",
			"h/l, ←/→          Move between form actions",
			"enter             Edit field or activate action",
			"ctrl+s            Save contact",
		}
	case modeBooks:
		return []string{
			"j/k, ↑/↓          Move through addressbooks and actions",
			"tab               Focus actions",
			"enter             Apply focused action",
			"esc, q, ctrl+c    Cancel",
		}
	case modeConfirm:
		return []string{
			"tab, ←/→          Choose Confirm / Cancel",
			"enter, y          Activate confirmation",
			"esc, q, n, ctrl+c Cancel operation",
		}
	case modeEmailMatches:
		return []string{
			"j/k, ↑/↓          Move through matches and actions",
			"tab               Focus actions",
			"enter             Apply focused action",
			"esc, q, ctrl+c    Cancel",
		}
	case modeConflict:
		return []string{
			"←/→               Change conflict value",
			"tab, ↑/↓          Move to actions",
			"enter             Apply focused action",
			"esc, q, ctrl+c    Cancel merge",
		}
	}
	if m.focusedPane == paneDetails {
		return []string{
			"h, ←, esc         Return to Contacts",
			"q                 Exit",
			"j/k, ↑/↓          Scroll contact details",
			"tab / shift-tab   Change pane",
			"enter, e          Edit contact",
			"n                 New contact",
		}
	}
	lines := []string{
		"esc, q, ctrl+c    Exit",
		"j/k, ↑/↓          Next / previous contact",
		"ctrl+f/b          Page down / page up",
		"tab / shift-tab   Change pane",
		"l, →, enter       Focus Details",
		"[ / ]             Previous / next addressbook",
		"/                 Search contacts",
		"e                 Edit contact",
		"n                 New contact",
		"space             Select/unselect contact",
	}
	if m.canCopy() {
		lines = append(lines, "c                 Copy selected contacts")
	}
	if m.canMove() {
		lines = append(lines, "m                 Move selected contacts")
	}
	if m.canMerge() {
		lines = append(lines, "M                 Merge selected contacts")
	}
	if m.selectedCount() > 0 {
		lines = append(lines, "d                 Delete selected contacts")
	}
	return lines
}

func (m *model) renderHelpOverlay(height int) string {
	width := dialogBodyWidth(m.width)
	content := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.Accent.Render("Shortcuts"),
		"",
		lipgloss.NewStyle().Width(width).MaxWidth(width).Render(strings.Join(m.helpLines(), "\n")),
	)
	outerWidth := dialogOuterWidth(m.width)
	box := m.styles.HelpOverlay.
		Width(dialogStyleWidth(m.width)).
		MaxWidth(outerWidth).
		Render(content)
	if m.width <= 0 || height <= 0 {
		return box
	}
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func editorPopupView(form *huh.Form, width, height int) string {
	if form == nil {
		return ""
	}
	return form.WithWidth(max(1, width)).WithHeight(max(1, height)).WithShowHelp(false).WithShowErrors(true).View()
}

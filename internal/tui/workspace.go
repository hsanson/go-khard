package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/contact"
)

const (
	detailActionStatusDuration = 2 * time.Second
	detailPathKey              = "path"
)

type detailAction uint8

const (
	detailActionNone detailAction = iota
	detailActionMail
	detailActionOpen
	detailActionCopy
)

type detailActionStatus struct {
	action       detailAction
	key, value   string
	index        int
	done, failed bool
}

type detailActionResultMsg struct {
	id  uint64
	err error
}

type detailActionExpiredMsg struct{ id uint64 }

func runDetailAction(action detailAction, value string, id uint64) tea.Cmd {
	return func() tea.Msg {
		var err error
		if action == detailActionCopy {
			err = clipboard.WriteAll(value)
		} else {
			var target string
			target, err = detailActionTarget(action, value)
			if err == nil {
				err = openSystemTarget(target)
			}
		}
		return detailActionResultMsg{id: id, err: err}
	}
}

func detailActionTarget(action detailAction, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch action {
	case detailActionMail:
		if err := validEmail(value); err != nil {
			return "", err
		}
		return "mailto:" + value, nil
	case detailActionOpen:
		target, err := url.ParseRequestURI(value)
		if err != nil {
			return "", fmt.Errorf("invalid web page URL")
		}
		scheme := strings.ToLower(target.Scheme)
		if target.Host == "" || scheme != "http" && scheme != "https" {
			return "", fmt.Errorf("invalid web page URL")
		}
		return value, nil
	default:
		return "", fmt.Errorf("unsupported detail action")
	}
}

func openSystemTarget(target string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", target).Run()
	case "darwin":
		return exec.Command("open", target).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Run()
	default:
		return fmt.Errorf("opening links is unsupported on %s", runtime.GOOS)
	}
}

func expireDetailAction(id uint64) tea.Cmd {
	return tea.Tick(detailActionStatusDuration, func(time.Time) tea.Msg {
		return detailActionExpiredMsg{id: id}
	})
}

func (m *model) currentPath() string {
	if current := m.current(); current != nil {
		return current.Path
	}
	return ""
}

func (m *model) clearDetailAction() {
	m.detailActionStatusID++
	m.detailStatus = detailActionStatus{}
}

func (m *model) startDetailAction(action detailAction, key string, index int, value string) tea.Cmd {
	m.clearDetailAction()
	m.detailStatus = detailActionStatus{action: action, key: key, index: index, value: value}
	return runDetailAction(action, value, m.detailActionStatusID)
}

func (m *model) resetDetailState() {
	m.detailScroll = 0
	m.clearDetailAction()
}

func (m *model) resetDetailsIfContactChanged(previous string) {
	if m.currentPath() != previous {
		m.resetDetailState()
	}
}

func (m *model) moveWorkspaceFocus() {
	if m.focusedPane == paneContacts {
		m.focusedPane = paneDetails
	} else {
		m.focusedPane = paneContacts
	}
}

func (m *model) canCopy() bool {
	return m.selectedCount() > 0 && len(m.books) > 0
}

func (m *model) canMove() bool {
	if m.selectedCount() == 0 {
		return false
	}
	for _, selected := range m.selectedTargets() {
		for _, book := range m.books {
			if book.Path != selected.Book.Path {
				return true
			}
		}
	}
	return false
}

func (m *model) canMerge() bool { return m.selectedCount() > 1 }

func (m *model) workspaceView(mainHeight int) string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	if mainHeight <= 0 {
		mainHeight = 30
	}
	workspaceWidth := max(8, width-2)
	panelHeight := max(4, mainHeight-2)
	leftWidth := max(4, workspaceWidth/3)
	leftWidth = min(leftWidth, workspaceWidth-4)
	rightWidth := workspaceWidth - leftWidth

	leftX, panelY := 1, 1
	rightX := leftX + leftWidth
	left := m.renderContactsPane(leftWidth, panelHeight, leftX, panelY)
	right := m.renderRightPane(rightWidth, panelHeight, rightX, panelY)
	return m.styles.Container.Render(lipgloss.JoinHorizontal(lipgloss.Top, left, right))
}

func (m *model) renderPane(content string, width, height int, focused bool) string {
	style := m.styles.MainPanel
	if focused {
		style = m.styles.FocusedPanel
	}
	return style.
		Width(max(2, width-2)).
		Height(max(1, height-2)).
		MaxWidth(width).
		MaxHeight(height).
		Render(content)
}

func (m *model) renderContactsPane(width, height, x, y int) string {
	m.addMouseHit(mouseHit{rect: mouseRect{x: x, y: y, width: width, height: height}, kind: mouseContactsPane})
	innerWidth, innerHeight := max(1, width-4), max(1, height-2)
	content := m.contactListView(innerWidth, innerHeight, x+2, y+1)
	focused := (m.mode == modeList || m.mode == modeSearch) && m.focusedPane == paneContacts
	return m.renderPane(content, width, height, focused)
}

func (m *model) renderRightPane(width, height, x, y int) string {
	m.addMouseHit(mouseHit{rect: mouseRect{x: x, y: y, width: width, height: height}, kind: mouseDetailsPane})
	innerWidth, innerHeight := max(1, width-4), max(1, height-2)
	contentX, contentY := x+2, y+1
	var content string
	switch m.mode {
	case modeForm:
		content = m.renderSized(innerWidth, innerHeight, contentX, contentY, m.formView)
	case modeBooks:
		content = m.renderSized(innerWidth, innerHeight, contentX, contentY, m.bookView)
	case modeConfirm:
		content = m.renderSized(innerWidth, innerHeight, contentX, contentY, m.confirmView)
	case modeConflict:
		content = m.renderSized(innerWidth, innerHeight, contentX, contentY, m.conflictView)
	default:
		content = m.contactDetailsView(innerWidth, innerHeight, contentX, contentY)
	}
	focused := m.mode != modeList && m.mode != modeSearch || m.focusedPane == paneDetails
	return m.renderPane(content, width, height, focused)
}

func (m *model) renderSized(width, height, x, y int, render func() string) string {
	oldWidth, oldHeight := m.width, m.height
	firstHit := 0
	if m.mouse != nil {
		firstHit = len(m.mouse.hits)
	}
	m.width = width
	footerHeight := lipgloss.Height(m.footerView())
	m.height = height
	if footerHeight > 0 {
		m.height += footerHeight + 1
	}
	view := render()
	m.width, m.height = oldWidth, oldHeight
	if m.mouse != nil {
		for i := firstHit; i < len(m.mouse.hits); i++ {
			m.mouse.hits[i].rect.x += x
			m.mouse.hits[i].rect.y += y
		}
	}
	return view
}

func (m *model) contactListBodyHeight(height int) int {
	used := 2
	if m.message != "" {
		used++
	}
	if m.mode == modeSearch {
		used++
	}
	if m.canMove() || m.canCopy() || m.canMerge() {
		used += 2
	}
	return max(1, height-used)
}

func (m *model) listPaneHeight() int {
	mainHeight := m.mainHeight()
	return max(1, max(4, mainHeight-2)-2)
}

func (m *model) contactListView(width, height, x, y int) string {
	lines := []string{m.styles.PanelTitle.Render("Contacts")}
	stats := fmt.Sprintf(" %d   %d   %s", len(m.visible), m.selectedCount(), m.filterBookName())
	lines = append(lines, m.styles.Subtle.Render(stats))
	bookPrefix := fmt.Sprintf(" %d   %d   ", len(m.visible), m.selectedCount())
	m.addMouseHit(mouseHit{
		rect: mouseRect{x: x + ansi.StringWidth(bookPrefix), y: y + 1, width: max(1, ansi.StringWidth(m.filterBookName())), height: 1},
		kind: mouseAddressbook,
	})
	if m.message != "" {
		message := m.styles.Subtle.Render(m.message)
		if m.messageErr {
			message = m.styles.Error.Render(m.message)
		}
		lines = append(lines, message)
	}
	if m.mode == modeSearch {
		lines = append(lines, m.search.View())
	}

	bodyStart := len(lines)
	bodyHeight := m.contactListBodyHeight(height)
	end := min(len(m.visible), m.offset+bodyHeight)
	for i := m.offset; i < end; i++ {
		entry := m.visible[i]
		cursor, selected := " ", " "
		if i == m.cursor {
			cursor = "›"
		}
		if m.selected[entry.Path] {
			selected = "✓"
		}
		line := cursor + selected + " " + clip(entry.Name(), max(1, width-3))
		if i == m.cursor {
			line = m.styles.SelectedRow.Width(width).Render(line)
		}
		m.addMouseHit(mouseHit{rect: mouseRect{x: x, y: y + len(lines), width: width, height: 1}, kind: mouseContact, index: i})
		lines = append(lines, line)
	}
	if len(m.visible) == 0 {
		lines = append(lines, m.styles.Subtle.Render("No matches"))
	}
	for len(lines) < bodyStart+bodyHeight {
		lines = append(lines, "")
	}

	type action struct {
		label string
		kind  mouseTarget
		show  bool
	}
	actions := []action{
		{label: "Move", kind: mouseMoveAction, show: m.canMove()},
		{label: "Copy", kind: mouseCopyAction, show: m.canCopy()},
		{label: "Merge", kind: mouseMergeAction, show: m.canMerge()},
	}
	if m.canMove() || m.canCopy() || m.canMerge() {
		lines = append(lines, "")
		var row strings.Builder
		buttonX := 0
		for _, action := range actions {
			if !action.show {
				continue
			}
			if row.Len() > 0 {
				row.WriteString("  ")
				buttonX = lipgloss.Width(row.String())
			}
			button := m.styles.SecondaryButton.Render(action.label)
			m.addMouseHit(mouseHit{rect: mouseRect{x: x + buttonX, y: y + len(lines), width: lipgloss.Width(button), height: 1}, kind: action.kind})
			row.WriteString(button)
		}
		lines = append(lines, row.String())
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines[:min(len(lines), height)], "\n"))
}

type detailButtonHit struct {
	line, x, width int
	kind           mouseTarget
	key            string
	index          int
}

func (m *model) contactDetailsView(width, height, x, y int) string {
	title := m.styles.PanelTitle.Render("Details")
	current := m.current()
	if current == nil {
		body := m.styles.Subtle.Render("No contact selected")
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(lipgloss.JoinVertical(lipgloss.Left, title, body))
	}

	rows := []editorRow{
		{section: true, label: sectionIcons["storage"] + "  Address book"},
		{key: "addressbook", label: "Addressbook", value: current.Book.Name()},
	}
	rows = append(rows, contactFieldRows(current.Card, false)...)
	lines := make([]string, 0, len(rows)*2+4)
	var buttons []detailButtonHit
	for _, row := range rows {
		if row.section {
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
				lines = append(lines, "")
			}
			lines = append(lines, m.styles.Accent.Render(clip(strings.TrimSpace(row.label), width)))
			continue
		}
		if row.key == vcard.FieldNote {
			lines = append(lines, detailNoteLines(m.styles, row.value, width)...)
			continue
		}
		switch row.key {
		case vcard.FieldEmail:
			row.label = detailLabel("Email", current.Card[row.key][row.index])
		case vcard.FieldAddress:
			row.label = detailLabel("Address", current.Card[row.key][row.index])
		}
		lines = append(lines, m.styles.detailLine("", row.label, row.value, width))
		if actionLine, actionButtons := m.detailFieldActionLine(row, width); actionLine != "" {
			for i := range actionButtons {
				actionButtons[i].line = len(lines)
			}
			buttons = append(buttons, actionButtons...)
			lines = append(lines, actionLine)
		}
	}

	if current.Path != "" {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, m.styles.Accent.Render(sectionIcons["file"]+"  vCard file"))
		prefix := m.styles.detailPrefix("", "Path")
		copyLabel, copyStyle := m.detailActionButton(detailActionCopy, detailPathKey, 0, current.Path, " Copy")
		button := copyStyle.Render(copyLabel)
		buttons = append(buttons, detailButtonHit{
			line:  len(lines),
			x:     lipgloss.Width(prefix),
			width: lipgloss.Width(button),
			kind:  mousePathCopy,
			key:   detailPathKey,
		})
		lines = append(lines, prefix+button)
	}

	bodyHeight := max(1, height-1)
	m.detailScroll = min(m.detailScroll, max(0, len(lines)-bodyHeight))
	for _, button := range buttons {
		if button.line < m.detailScroll || button.line >= m.detailScroll+bodyHeight {
			continue
		}
		m.addMouseHit(mouseHit{
			rect:  mouseRect{x: x + button.x, y: y + 1 + button.line - m.detailScroll, width: button.width, height: 1},
			kind:  button.kind,
			key:   button.key,
			index: button.index,
		})
	}
	body := renderDetailLines(lines, m.detailScroll, bodyHeight)
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(lipgloss.JoinVertical(lipgloss.Left, title, body))
}

func (m *model) detailFieldActionLine(row editorRow, width int) (string, []detailButtonHit) {
	if strings.TrimSpace(row.value) == "" {
		return "", nil
	}
	type action struct {
		kind   mouseTarget
		action detailAction
		label  string
	}
	var actions []action
	switch row.key {
	case vcard.FieldEmail:
		actions = []action{
			{kind: mouseDetailMail, action: detailActionMail, label: "󰇮 Mail"},
			{kind: mouseDetailCopy, action: detailActionCopy, label: " Copy"},
		}
	case vcard.FieldURL:
		actions = []action{
			{kind: mouseDetailOpen, action: detailActionOpen, label: "󰖟 Open"},
			{kind: mouseDetailCopy, action: detailActionCopy, label: " Copy"},
		}
	default:
		return "", nil
	}

	var line strings.Builder
	var buttons []detailButtonHit
	for _, action := range actions {
		if line.Len() > 0 {
			line.WriteString("  ")
		}
		buttonX := lipgloss.Width(line.String())
		label, style := m.detailActionButton(action.action, row.key, row.index, row.value, action.label)
		button := style.Render(label)
		buttons = append(buttons, detailButtonHit{x: buttonX, width: lipgloss.Width(button), kind: action.kind, key: row.key, index: row.index})
		line.WriteString(button)
	}
	indentWidth := min(lipgloss.Width(m.styles.detailPrefix("", row.label)), max(0, width-lipgloss.Width(line.String())))
	for i := range buttons {
		buttons[i].x += indentWidth
	}
	return strings.Repeat(" ", indentWidth) + line.String(), buttons
}

func (m *model) detailActionButton(action detailAction, key string, index int, value, label string) (string, lipgloss.Style) {
	style := m.styles.SecondaryButton
	status := m.detailStatus
	if !status.done || status.action != action || status.key != key || status.index != index || status.value != value {
		return label, style
	}
	if status.failed {
		return label + " Failed", m.styles.DangerButton
	}
	if action == detailActionCopy {
		return " Copied", style
	}
	return label, style
}

func (styles Styles) detailPrefix(glyph, label string) string {
	const iconWidth, labelWidth = 2, 13
	icon := strings.Repeat(" ", iconWidth)
	if glyph != "" {
		icon = styles.Accent.Render(glyph)
		icon += strings.Repeat(" ", max(0, iconWidth-lipgloss.Width(icon)))
	}
	return icon + " " + styles.Accent.Render(padDetailRight(label, labelWidth)) + styles.Disabled.Render(": ")
}

func (styles Styles) detailLine(glyph, label, value string, width int) string {
	prefix := styles.detailPrefix(glyph, label)
	budget := max(1, width-lipgloss.Width(prefix))
	return prefix + styles.Value.Render(ansi.Truncate(value, budget, "…"))
}

func detailNoteLines(styles Styles, note string, width int) []string {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	width = max(1, width)
	indent := "   "
	if width <= len(indent) {
		indent = indent[:max(0, width-1)]
	}
	wrapped := strings.Split(ansi.Wrap(note, max(1, width-len(indent)), ""), "\n")
	lines := []string{styles.detailLine("󰎞", "Note", "", width)}
	for _, line := range wrapped {
		lines = append(lines, indent+line)
	}
	return lines
}

func renderDetailLines(lines []string, scroll, height int) string {
	if height <= 0 || len(lines) == 0 {
		return ""
	}
	scroll = min(max(0, scroll), max(0, len(lines)-height))
	end := min(len(lines), scroll+height)
	return lipgloss.NewStyle().Height(height).MaxHeight(height).Render(strings.Join(lines[scroll:end], "\n"))
}

func padDetailRight(value string, width int) string {
	if lipgloss.Width(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-lipgloss.Width(value))
}

func selectedContactByPath(contacts []contact.Contact, path string) int {
	for i := range contacts {
		if contacts[i].Path == path {
			return i
		}
	}
	return -1
}

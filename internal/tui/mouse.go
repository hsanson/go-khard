package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type mouseTarget uint8

const (
	mouseAddressbook mouseTarget = iota + 1
	mouseContact
	mouseEditorRow
	mouseBook
	mouseEmailMatch
	mouseDialogAction
	mouseDateDay
	mouseDatePreviousYear
	mouseDatePreviousMonth
	mouseDateNextMonth
	mouseDateNextYear
	mouseDateToday
	mouseDateClear
)

type mouseRect struct {
	x, y, width, height int
}

type mouseHit struct {
	rect  mouseRect
	kind  mouseTarget
	index int
	focus dialogFocus
	day   time.Time
}

type mouseState struct {
	hits []mouseHit
}

func (s *mouseState) reset() {
	s.hits = s.hits[:0]
}

func (s *mouseState) add(hit mouseHit) {
	if hit.rect.width > 0 && hit.rect.height > 0 {
		s.hits = append(s.hits, hit)
	}
}

func (s *mouseState) at(x, y int) (mouseHit, bool) {
	for i := len(s.hits) - 1; i >= 0; i-- {
		hit := s.hits[i]
		if x >= hit.rect.x && x < hit.rect.x+hit.rect.width && y >= hit.rect.y && y < hit.rect.y+hit.rect.height {
			return hit, true
		}
	}
	return mouseHit{}, false
}

func (m *model) addMouseHit(hit mouseHit) {
	if m.mouse != nil {
		m.mouse.add(hit)
	}
}

func cycleDialogAction(focus *dialogFocus, delta int, hasDelete bool) {
	focuses := []dialogFocus{dialogFocusControl, dialogFocusPrimary, dialogFocusCancel}
	if hasDelete {
		focuses = append(focuses, dialogFocusDelete)
	}
	index := 0
	for i, candidate := range focuses {
		if candidate == *focus {
			index = i
			break
		}
	}
	*focus = focuses[(index+delta+len(focuses))%len(focuses)]
}

func selectDialogAction(focus *dialogFocus, selected dialogFocus) {
	if *focus != dialogFocusControl {
		*focus = selected
	}
}

func moveDialogCursor(focus *dialogFocus, cursor *int, total, delta int, hasDelete bool) {
	if total <= 0 {
		return
	}
	if *focus != dialogFocusControl {
		cycleDialogAction(focus, delta, hasDelete)
		if *focus == dialogFocusControl {
			if delta > 0 {
				*cursor = 0
			} else {
				*cursor = total - 1
			}
		}
		return
	}
	next := *cursor + delta
	if next >= 0 && next < total {
		*cursor = next
		return
	}
	if delta > 0 {
		*focus = dialogFocusPrimary
	} else if hasDelete {
		*focus = dialogFocusDelete
	} else {
		*focus = dialogFocusCancel
	}
}

func (m *model) updateMouse(event tea.MouseEvent) (tea.Model, tea.Cmd) {
	if event.Button == tea.MouseButtonWheelUp {
		return m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if event.Button == tea.MouseButtonWheelDown {
		return m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if event.Action != tea.MouseActionPress || event.Button != tea.MouseButtonLeft || m.showHelp || m.mouse == nil {
		return m, nil
	}
	hit, ok := m.mouse.at(event.X, event.Y)
	if !ok {
		return m, nil
	}
	activeDialog := (m.mode == modeForm && (m.form.activeForm != nil || m.form.datePicker != nil)) ||
		(m.mode == modeConflict && m.conflictForm != nil) ||
		m.mode == modeBooks || m.mode == modeConfirm || m.mode == modeEmailMatches
	dateTarget := hit.kind >= mouseDateDay && hit.kind <= mouseDateClear
	if activeDialog && hit.kind != mouseDialogAction && hit.kind != mouseBook && hit.kind != mouseEmailMatch && !dateTarget {
		return m, nil
	}

	switch hit.kind {
	case mouseAddressbook:
		m.cycleAddressbook(1)
	case mouseContact:
		if hit.index < 0 || hit.index >= len(m.visible) {
			break
		}
		m.cursor = hit.index
		if event.Ctrl {
			contact := m.visible[hit.index]
			m.selected[contact.Path] = !m.selected[contact.Path]
			break
		}
		m.startForm(&m.visible[hit.index], nil)
	case mouseEditorRow:
		rows := m.editorRows()
		if hit.index < 0 || hit.index >= len(rows) || !selectableEditorRow(rows[hit.index]) {
			break
		}
		m.form.cursor = hit.index
		return m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	case mouseBook:
		if hit.index >= 0 && hit.index < len(m.books) {
			m.bookCursor = hit.index
			m.dialogFocus = dialogFocusControl
		}
	case mouseEmailMatch:
		if hit.index >= 0 && hit.index <= len(m.emailMatches) {
			m.emailMatchCursor = hit.index
			m.dialogFocus = dialogFocusControl
		}
	case mouseDialogAction:
		return m.activateMouseDialog(hit.focus)
	case mouseDateDay, mouseDatePreviousYear, mouseDatePreviousMonth, mouseDateNextMonth, mouseDateNextYear, mouseDateToday, mouseDateClear:
		if picker := m.form.datePicker; picker != nil {
			switch hit.kind {
			case mouseDateDay:
				picker.selectDate(hit.day)
			case mouseDatePreviousYear:
				picker.moveMonth(-12)
			case mouseDatePreviousMonth:
				picker.moveMonth(-1)
			case mouseDateNextMonth:
				picker.moveMonth(1)
			case mouseDateNextYear:
				picker.moveMonth(12)
			case mouseDateToday:
				picker.selectDate(time.Now())
			case mouseDateClear:
				picker.clear()
			}
		}
	}
	return m, nil
}

func (m *model) activateMouseDialog(focus dialogFocus) (tea.Model, tea.Cmd) {
	if m.mode == modeForm && (m.form.activeForm != nil || m.form.datePicker != nil) {
		m.form.dialogFocus = focus
		return m.activateEditorDialog()
	}
	if m.mode == modeConflict && m.conflictForm != nil {
		m.dialogFocus = focus
		if focus == dialogFocusCancel {
			return m.cancelConflict()
		}
		return m.applyConflict()
	}
	m.dialogFocus = focus
	switch m.mode {
	case modeBooks:
		return m.activateBookDialog()
	case modeConfirm:
		return m.activateConfirmation()
	case modeEmailMatches:
		return m.activateEmailMatchDialog()
	}
	return m, nil
}

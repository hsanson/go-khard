package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type contactDatePicker struct {
	cursor       time.Time
	month        time.Time
	original     string
	legacyStatus string
	selected     bool
	cleared      bool
}

func newContactDatePicker(value string, now time.Time) *contactDatePicker {
	today := dateOnly(now)
	picker := &contactDatePicker{cursor: today, month: firstOfMonth(today), original: strings.TrimSpace(value)}
	for _, layout := range []string{"2006-01-02", "20060102"} {
		if parsed, err := time.Parse(layout, picker.original); err == nil {
			picker.cursor = dateOnly(parsed)
			picker.month = firstOfMonth(picker.cursor)
			picker.selected = true
			return picker
		}
	}
	if month, day, ok := parseVCardMonthDay(picker.original); ok {
		lastDay := time.Date(today.Year(), month+1, 0, 0, 0, 0, 0, time.UTC).Day()
		picker.cursor = time.Date(today.Year(), month, min(day, lastDay), 0, 0, 0, 0, time.UTC)
		picker.month = firstOfMonth(picker.cursor)
		picker.legacyStatus = fmt.Sprintf("Date: %s (year unspecified)", time.Date(2000, month, day, 0, 0, 0, 0, time.UTC).Format("Jan 2"))
	} else if picker.original != "" {
		picker.legacyStatus = "Existing: " + picker.original
	}
	return picker
}

func parseVCardMonthDay(value string) (time.Month, int, bool) {
	value = strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(value), "--"), "-", "")
	if len(value) != 4 {
		return 0, 0, false
	}
	parsed, err := time.Parse("0102", value)
	if err != nil {
		return 0, 0, false
	}
	return parsed.Month(), parsed.Day(), true
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func firstOfMonth(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func sameDate(left, right time.Time) bool {
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}

func (p *contactDatePicker) value() string {
	if p.cleared {
		return ""
	}
	if p.selected {
		return p.cursor.Format("2006-01-02")
	}
	return p.original
}

func (p *contactDatePicker) selectDate(value time.Time) {
	p.cursor = dateOnly(value)
	p.month = firstOfMonth(p.cursor)
	p.selected = true
	p.cleared = false
	p.legacyStatus = ""
}

func (p *contactDatePicker) clear() {
	p.selected = false
	p.cleared = true
	p.legacyStatus = ""
}

func (p *contactDatePicker) moveDays(days int) {
	p.selectDate(p.cursor.AddDate(0, 0, days))
}

func (p *contactDatePicker) moveMonth(months int) {
	day := p.cursor.Day()
	nextMonth := firstOfMonth(p.cursor).AddDate(0, months, 0)
	day = min(day, nextMonth.AddDate(0, 1, -1).Day())
	p.selectDate(time.Date(nextMonth.Year(), nextMonth.Month(), day, 0, 0, 0, 0, time.UTC))
}

func (p *contactDatePicker) update(msg tea.KeyMsg) {
	switch msg.String() {
	case "space", " ":
		p.clear()
	case "[", "pgup":
		p.moveMonth(-1)
	case "]", "pgdown":
		p.moveMonth(1)
	case "left", "h":
		p.moveDays(-1)
	case "right", "l":
		p.moveDays(1)
	case "up", "k":
		p.moveDays(-7)
	case "down", "j":
		p.moveDays(7)
	case "t":
		p.selectDate(time.Now())
	}
}

func (p *contactDatePicker) render() (string, []mouseHit) {
	header := fmt.Sprintf("%-18s ‹ ›", p.month.Format("January 2006"))
	lines := []string{
		accent.Render(header),
		dim.Render("Mo Tu We Th Fr Sa Su"),
	}
	hits := []mouseHit{
		{rect: mouseRect{x: 19, y: 0, width: 1, height: 1}, kind: mouseDatePreviousMonth},
		{rect: mouseRect{x: 21, y: 0, width: 1, height: 1}, kind: mouseDateNextMonth},
	}
	firstWeekday := (int(p.month.Weekday()) + 6) % 7
	monthEnd := p.month.AddDate(0, 1, -1).Day()
	for week := range 6 {
		cells := make([]string, 0, 7)
		for weekday := range 7 {
			day := week*7 + weekday - firstWeekday + 1
			if day < 1 || day > monthEnd {
				cells = append(cells, "  ")
				continue
			}
			date := time.Date(p.month.Year(), p.month.Month(), day, 0, 0, 0, 0, time.UTC)
			cell := fmt.Sprintf("%2d", day)
			if sameDate(date, p.cursor) {
				cell = lipgloss.NewStyle().Background(lipgloss.Color("117")).Foreground(lipgloss.Color("232")).Bold(true).Render(cell)
			}
			hits = append(hits, mouseHit{
				rect: mouseRect{x: weekday * 3, y: len(lines), width: 2, height: 1},
				kind: mouseDateDay,
				day:  date,
			})
			cells = append(cells, cell)
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	lines = append(lines, "")
	controlsY := len(lines)
	today, clear := "[ Today ]", "[ Clear ]"
	lines = append(lines, today+"  "+clear)
	hits = append(hits,
		mouseHit{rect: mouseRect{x: 0, y: controlsY, width: len(today), height: 1}, kind: mouseDateToday},
		mouseHit{rect: mouseRect{x: len(today) + 2, y: controlsY, width: len(clear), height: 1}, kind: mouseDateClear},
	)
	status := "Date: —"
	switch {
	case p.cleared:
	case p.selected:
		status = "Date: " + p.cursor.Format("Jan 2, 2006")
	case p.legacyStatus != "":
		status = p.legacyStatus
	}
	lines = append(lines, "", clip(status, 60))
	return strings.Join(lines, "\n"), hits
}

func (m *model) updateActiveDatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q", "ctrl+c":
		m.cancelEditorDialog()
		return m, nil
	case "tab":
		cycleDialogAction(&m.form.dialogFocus, 1, false)
		return m, nil
	case "shift+tab":
		cycleDialogAction(&m.form.dialogFocus, -1, false)
		return m, nil
	case "left", "h":
		if m.form.dialogFocus != dialogFocusControl {
			selectDialogAction(&m.form.dialogFocus, dialogFocusPrimary)
			return m, nil
		}
	case "right", "l":
		if m.form.dialogFocus != dialogFocusControl {
			selectDialogAction(&m.form.dialogFocus, dialogFocusCancel)
			return m, nil
		}
	case "enter":
		if m.form.dialogFocus == dialogFocusControl {
			m.submitEditorDialog()
			return m, nil
		}
		return m.activateEditorDialog()
	case " ":
		if m.form.dialogFocus != dialogFocusControl {
			return m.activateEditorDialog()
		}
	}
	if m.form.dialogFocus == dialogFocusControl {
		m.form.datePicker.update(key)
	}
	return m, nil
}

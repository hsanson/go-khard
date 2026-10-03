package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emersion/go-vcard"
)

func TestContactDatePickerPreservesOptionalAndLegacyValues(t *testing.T) {
	now := time.Date(2025, time.January, 10, 12, 0, 0, 0, time.UTC)

	empty := newContactDatePicker("", now)
	if got := empty.value(); got != "" {
		t.Fatalf("empty picker value = %q", got)
	}
	empty.moveDays(1)
	if got := empty.value(); got != "2025-01-11" {
		t.Fatalf("selected empty picker value = %q", got)
	}
	empty.clear()
	if got := empty.value(); got != "" {
		t.Fatalf("cleared picker value = %q", got)
	}

	yearless := newContactDatePicker("--01-19", now)
	if got := yearless.value(); got != "--01-19" {
		t.Fatalf("untouched yearless value = %q", got)
	}
	view, _ := yearless.render(DefaultStyles())
	if !strings.Contains(view, "Jan 19 (year unspecified)") {
		t.Fatalf("yearless status missing:\n%s", view)
	}
	yearless.moveDays(1)
	if got := yearless.value(); got != "2025-01-20" {
		t.Fatalf("selected yearless value = %q", got)
	}

	unknown := newContactDatePicker("text=unknown", now)
	view, _ = unknown.render(DefaultStyles())
	if unknown.value() != "text=unknown" || !strings.Contains(view, "Existing: text=unknown") {
		t.Fatalf("unsupported value was not preserved:\n%s", view)
	}

	compact := newContactDatePicker("20100119", now)
	if got := compact.value(); got != "2010-01-19" {
		t.Fatalf("compact date value = %q", got)
	}
}

func TestContactDatePickerNavigatesMonthsAndYears(t *testing.T) {
	month := newContactDatePicker("2025-01-31", time.Time{})
	month.update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if got := month.value(); got != "2025-02-28" {
		t.Fatalf("Ctrl-J date = %q", got)
	}
	month.update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if got := month.value(); got != "2025-01-28" {
		t.Fatalf("Ctrl-K date = %q", got)
	}

	year := newContactDatePicker("2024-02-29", time.Time{})
	year.update(tea.KeyMsg{Type: tea.KeyCtrlH})
	if got := year.value(); got != "2023-02-28" {
		t.Fatalf("Ctrl-H date = %q", got)
	}
	year.update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if got := year.value(); got != "2024-02-28" {
		t.Fatalf("Ctrl-L date = %q", got)
	}
}

func TestBirthdayUsesCalendarAndAppliesSelectedDate(t *testing.T) {
	m := &model{mode: modeForm, width: 100, height: 40, form: formState{card: make(vcard.Card)}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldBirthday {
			m.form.cursor = i
			break
		}
	}
	_ = m.openEditorPopup(rows)
	if m.form.datePicker == nil || m.form.activeForm != nil {
		t.Fatalf("Birthday editor is not the calendar picker: %#v", m.form)
	}
	m.form.datePicker.selectDate(time.Date(1815, time.December, 10, 0, 0, 0, 0, time.UTC))
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.form.datePicker != nil {
		t.Fatal("Enter did not close the Birthday calendar")
	}
	if got := m.form.card.Value(vcard.FieldBirthday); got != "1815-12-10" {
		t.Fatalf("Birthday after Enter = %q", got)
	}
}

func TestCalendarMouseSelectsAndClearsDate(t *testing.T) {
	m := &model{
		mode: modeForm, width: 100, height: 40, mouse: &mouseState{},
		form: formState{
			card: make(vcard.Card), activeRow: editorRow{key: vcard.FieldBirthday, label: "Birthday"},
			datePicker: newContactDatePicker("", time.Date(2025, time.January, 10, 0, 0, 0, 0, time.UTC)),
		},
	}
	_ = m.View()
	var day, clear mouseHit
	for _, hit := range m.mouse.hits {
		switch {
		case hit.kind == mouseDateDay && hit.day.Day() == 19:
			day = hit
		case hit.kind == mouseDateClear:
			clear = hit
		}
	}
	if day.kind == 0 || clear.kind == 0 {
		t.Fatalf("calendar mouse targets missing: %#v", m.mouse.hits)
	}
	_, _ = m.updateMouse(tea.MouseEvent{X: day.rect.x, Y: day.rect.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if got := m.form.datePicker.value(); got != "2025-01-19" {
		t.Fatalf("mouse-selected date = %q", got)
	}
	_, _ = m.updateMouse(tea.MouseEvent{X: clear.rect.x, Y: clear.rect.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if got := m.form.datePicker.value(); got != "" {
		t.Fatalf("mouse-cleared date = %q", got)
	}
}

func TestCalendarMouseNavigatesMonthsAndYears(t *testing.T) {
	m := &model{
		mode: modeForm, width: 100, height: 40, mouse: &mouseState{},
		form: formState{
			card: make(vcard.Card), activeRow: editorRow{key: vcard.FieldBirthday, label: "Birthday"},
			datePicker: newContactDatePicker("2024-02-29", time.Time{}),
		},
	}
	view := m.View()
	if !strings.Contains(view, "« ‹ › »") {
		t.Fatalf("calendar navigation controls missing:\n%s", view)
	}

	for _, step := range []struct {
		kind mouseTarget
		want string
	}{
		{mouseDateNextYear, "2025-02-28"},
		{mouseDatePreviousMonth, "2025-01-28"},
		{mouseDateNextMonth, "2025-02-28"},
		{mouseDatePreviousYear, "2024-02-28"},
	} {
		var target mouseHit
		for _, hit := range m.mouse.hits {
			if hit.kind == step.kind {
				target = hit
				break
			}
		}
		if target.kind == 0 {
			t.Fatalf("calendar mouse target %d missing: %#v", step.kind, m.mouse.hits)
		}
		_, _ = m.updateMouse(tea.MouseEvent{X: target.rect.x, Y: target.rect.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if got := m.form.datePicker.value(); got != step.want {
			t.Fatalf("mouse target %d date = %q, want %q", step.kind, got, step.want)
		}
	}
}

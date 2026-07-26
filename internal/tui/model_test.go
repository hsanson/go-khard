package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/contact"
)

func TestEscapeQuitsContactList(t *testing.T) {
	m := &model{mode: modeList}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("escape did not return a quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("escape command returned %T, want tea.QuitMsg", msg)
	}
}

func TestControlDDeletesAndPlainDIsUnbound(t *testing.T) {
	entry := contact.Contact{Card: vcard.Card{}}
	m := &model{mode: modeList, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.mode != modeList || m.op != opNone {
		t.Fatalf("plain d started deletion: mode=%v op=%v", m.mode, m.op)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.mode != modeConfirm || m.op != opDelete {
		t.Fatalf("ctrl+d did not start deletion: mode=%v op=%v", m.mode, m.op)
	}
}

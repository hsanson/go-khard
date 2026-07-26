package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/config"
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

func TestAddressbookFilterSelectsOneBookOrAll(t *testing.T) {
	one := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	two := config.Source{Path: "/tmp/two", Type: "addressbook", DisplayName: "Two"}
	contacts := []contact.Contact{
		{Path: "/tmp/one/a.vcf", Book: one, Card: vcard.Card{}},
		{Path: "/tmp/two/b.vcf", Book: two, Card: vcard.Card{}},
	}
	m := &model{
		mode:     modeList,
		books:    []config.Source{one, two},
		contacts: contacts,
		visible:  contacts,
		selected: map[string]bool{contacts[1].Path: true},
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if m.mode != modeAddressbookFilter || m.filterBookCursor != 0 {
		t.Fatalf("b did not open addressbook filter: mode=%v cursor=%d", m.mode, m.filterBookCursor)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeList || m.filterBook != one.Path || len(m.visible) != 1 || m.visible[0].Book.Path != one.Path {
		t.Fatalf("book filter not applied: mode=%v filter=%q visible=%#v", m.mode, m.filterBook, m.visible)
	}
	if m.selectedCount() != 0 {
		t.Fatal("addressbook filter retained hidden selections")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filterBook != "" || len(m.visible) != 2 {
		t.Fatalf("All filter not applied: filter=%q visible=%d", m.filterBook, len(m.visible))
	}
}

func TestTableCellUsesTerminalDisplayWidth(t *testing.T) {
	const width = 24
	ascii := tableCell("Cristobal Gomez", width)
	japanese := tableCell("CSメーリングリスト", width)
	if got := ansi.StringWidth(ascii); got != width {
		t.Fatalf("ASCII cell width = %d, want %d", got, width)
	}
	if got := ansi.StringWidth(japanese); got != width {
		t.Fatalf("Japanese cell width = %d, want %d", got, width)
	}
	if got := ansi.StringWidth(clip("非常に長い日本語の連絡先名", 12)); got > 12 {
		t.Fatalf("clipped Japanese name width = %d, want <= 12", got)
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

func TestAddEmailMatchSelectionStartsMerge(t *testing.T) {
	book := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	existingCard := make(vcard.Card)
	existingCard.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	senderCard := make(vcard.Card)
	senderCard.SetValue(vcard.FieldFormattedName, "Ada Byron")
	senderCard.AddValue(vcard.FieldEmail, "ada@example.net")
	existing := contact.Contact{Card: existingCard, Path: "/tmp/one/ada.vcf", Book: book}
	m := &model{
		mode: modeEmailMatches, books: []config.Source{book},
		emailMatches: []contact.Contact{existing},
		emailSender:  contact.Contact{Card: senderCard},
		selected:     map[string]bool{},
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeConflict && m.mode != modeForm {
		t.Fatalf("selecting match did not start merge: mode=%v", m.mode)
	}
	if len(m.mergeTargets) != 2 || m.mergeTargets[0].Path != existing.Path {
		t.Fatalf("merge targets = %#v", m.mergeTargets)
	}
}

func TestAddEmailCreateNewPrefillsSender(t *testing.T) {
	book := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	card.AddValue(vcard.FieldEmail, "ada@example.net")
	m := &model{
		mode: modeEmailMatches, books: []config.Source{book},
		emailMatches:     []contact.Contact{{Card: make(vcard.Card)}},
		emailMatchCursor: 1,
		emailSender:      contact.Contact{Card: card},
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeForm || m.form.editing != nil {
		t.Fatalf("Create new mode=%v editing=%#v", m.mode, m.form.editing)
	}
	if m.form.card.Value(vcard.FieldFormattedName) != "Ada Lovelace" ||
		m.form.card.Value(vcard.FieldEmail) != "ada@example.net" {
		t.Fatalf("prefilled card = %#v", m.form.card)
	}
}

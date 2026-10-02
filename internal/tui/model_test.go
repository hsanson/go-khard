package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
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

func TestTabCyclesAddressbooksAndPreservesSearch(t *testing.T) {
	one := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	two := config.Source{Path: "/tmp/two", Type: "addressbook", DisplayName: "Two"}
	contacts := []contact.Contact{
		{Path: "/tmp/one/a.vcf", Book: one, Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Ada"}}}},
		{Path: "/tmp/two/b.vcf", Book: two, Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Bob"}}}},
	}
	search := textinput.New()
	search.SetValue("ada")
	m := &model{
		mode: modeList, books: []config.Source{one, two}, contacts: contacts,
		selected: map[string]bool{contacts[1].Path: true}, search: search,
	}
	m.filter()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.filterBook != one.Path || len(m.visible) != 1 || m.visible[0].Path != contacts[0].Path {
		t.Fatalf("next addressbook: filter=%q visible=%#v", m.filterBook, m.visible)
	}
	if m.selectedCount() != 0 || m.search.Value() != "ada" {
		t.Fatalf("addressbook cycle selection=%d query=%q", m.selectedCount(), m.search.Value())
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.filterBook != "" || len(m.visible) != 1 {
		t.Fatalf("previous addressbook: filter=%q visible=%#v", m.filterBook, m.visible)
	}
}

func TestSearchFiltersAndNavigatesLive(t *testing.T) {
	search := textinput.New()
	contacts := []contact.Contact{
		{Path: "ada", Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Ada Lovelace"}}}},
		{Path: "alan", Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Alan Turing"}}}},
	}
	m := &model{mode: modeList, contacts: contacts, visible: contacts, selected: map[string]bool{}, search: search}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.mode != modeSearch || len(m.visible) != 2 {
		t.Fatalf("live search: mode=%v visible=%d", m.mode, len(m.visible))
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.cursor != 1 {
		t.Fatalf("Ctrl-J cursor = %d", m.cursor)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeList || m.search.Value() != "a" || len(m.visible) != 2 {
		t.Fatalf("kept search: mode=%v query=%q visible=%d", m.mode, m.search.Value(), len(m.visible))
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

func TestMoveAndDeleteRequireSelections(t *testing.T) {
	book := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	entry := contact.Contact{Path: "/tmp/one/a.vcf", Book: book, Card: make(vcard.Card)}
	m := &model{mode: modeList, books: []config.Source{book}, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.mode != modeList || m.op != opNone {
		t.Fatalf("unselected action: mode=%v op=%v", m.mode, m.op)
	}
	m.selected[entry.Path] = true
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.mode != modeBooks || m.op != opMove {
		t.Fatalf("m action: mode=%v op=%v", m.mode, m.op)
	}
	m.cancelBookOperation()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.mode != modeConfirm || m.op != opDelete {
		t.Fatalf("d action: mode=%v op=%v", m.mode, m.op)
	}
}

func TestMouseSelectsAndOpensContacts(t *testing.T) {
	book := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	entry := contact.Contact{Path: "/tmp/one/a.vcf", Book: book, Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Ada"}}}}
	m := &model{
		mode: modeList, width: 100, height: 20, books: []config.Source{book},
		contacts: []contact.Contact{entry}, visible: []contact.Contact{entry},
		selected: map[string]bool{}, mouse: &mouseState{},
	}
	_ = m.View()
	var contactHit mouseHit
	for _, hit := range m.mouse.hits {
		if hit.kind == mouseContact {
			contactHit = hit
			break
		}
	}
	_, _ = m.Update(tea.MouseMsg(tea.MouseEvent{
		X: contactHit.rect.x, Y: contactHit.rect.y, Ctrl: true,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	}))
	if !m.selected[entry.Path] || m.mode != modeList {
		t.Fatalf("Ctrl-click selected=%v mode=%v", m.selected[entry.Path], m.mode)
	}
	_ = m.View()
	for _, hit := range m.mouse.hits {
		if hit.kind == mouseContact {
			contactHit = hit
			break
		}
	}
	_, _ = m.Update(tea.MouseMsg(tea.MouseEvent{
		X: contactHit.rect.x, Y: contactHit.rect.y,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	}))
	if m.mode != modeForm || m.form.editing == nil {
		t.Fatalf("contact click mode=%v editing=%#v", m.mode, m.form.editing)
	}
}

func TestMouseAddressbookLabelCyclesFilter(t *testing.T) {
	one := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	two := config.Source{Path: "/tmp/two", Type: "addressbook", DisplayName: "Two"}
	m := &model{
		mode: modeList, width: 100, height: 20, books: []config.Source{one, two},
		selected: map[string]bool{}, mouse: &mouseState{},
	}
	_ = m.View()
	var bookHit mouseHit
	for _, hit := range m.mouse.hits {
		if hit.kind == mouseAddressbook {
			bookHit = hit
			break
		}
	}
	_, _ = m.Update(tea.MouseMsg(tea.MouseEvent{
		X: bookHit.rect.x, Y: bookHit.rect.y,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	}))
	if m.filterBook != one.Path {
		t.Fatalf("addressbook click filter = %q", m.filterBook)
	}
}

func TestNewContactUsesNAndPlainAIsUnbound(t *testing.T) {
	m := &model{mode: modeList}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.mode != modeList {
		t.Fatalf("plain a opened new contact form: mode=%v", m.mode)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.mode != modeForm || m.form.editing != nil {
		t.Fatalf("n did not open new contact form: mode=%v editing=%#v", m.mode, m.form.editing)
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

func TestMergeSelectionErrorIsRed(t *testing.T) {
	entry := contact.Contact{Card: make(vcard.Card)}
	m := &model{mode: modeList, width: 100, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}}
	_ = m.startMerge()
	if !m.messageErr {
		t.Fatal("merge selection failure was not marked as an error")
	}
	if want := errorStyle.Render("select at least two contacts to merge"); !strings.Contains(m.View(), want) {
		t.Fatalf("merge selection error is not rendered with error style:\n%s", m.View())
	}
}

func TestPlainEIsUnboundOnContactList(t *testing.T) {
	entry := contact.Contact{Card: make(vcard.Card)}
	m := &model{mode: modeList, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.mode != modeList {
		t.Fatalf("e opened contact editor: mode=%v", m.mode)
	}
}

func TestQReturnsFromContactEditorToList(t *testing.T) {
	m := &model{mode: modeList}
	m.startForm(nil, nil)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(*model)
	if cmd != nil || m.mode != modeList {
		t.Fatalf("q from editor: mode=%v command=%v", m.mode, cmd)
	}
}

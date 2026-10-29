package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/config"
	"github.com/hsanson/go-khard/internal/contact"
)

func TestContactDetailsShowCompleteReadOnlyContact(t *testing.T) {
	book := config.Source{Path: "/contacts/personal", Type: "addressbook", DisplayName: "Personal"}
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	card.SetValue(vcard.FieldBirthday, "18151210")
	card.Add(vcard.FieldEmail, &vcard.Field{
		Value:  "ada@example.test",
		Params: vcard.Params{vcard.ParamType: []string{" HOME ", "Internet", "home", ""}},
	})
	card.Add(vcard.FieldEmail, &vcard.Field{Value: "lovelace@example.test"})
	card.Add(vcard.FieldAddress, &vcard.Field{
		Value:  ";;1 Main Street;London;;;UK",
		Params: vcard.Params{vcard.ParamType: []string{"WORK"}},
	})
	card.Add(vcard.FieldURL, &vcard.Field{Value: "https://example.test"})
	note := "alpha beta gamma delta epsilon zeta eta theta iota kappa finalmarker"
	card.SetValue(vcard.FieldNote, note)
	entry := contact.Contact{Card: card, Path: "/contacts/personal/ada.vcf", Book: book}
	m := &model{visible: []contact.Contact{entry}, contacts: []contact.Contact{entry}, styles: DefaultStyles(), mouse: &mouseState{}}

	view := ansi.Strip(m.contactDetailsView(60, 80, 0, 0))
	for _, want := range []string{
		"Details", "Addressbook", "Personal", "Ada Lovelace", "1815-12-10",
		"Email (home, internet)", "ada@example.test", "lovelace@example.test",
		"Address (work)", "1 Main Street, London, UK", "https://example.test",
		"󰇮 Mail", "󰖟 Open", " Copy", "Note", "Path",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("details do not contain %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Email 2", "Address 1"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("details contain obsolete label %q:\n%s", unwanted, view)
		}
	}
	for _, target := range []mouseTarget{mouseDetailMail, mouseDetailOpen, mouseDetailCopy, mousePathCopy} {
		if !hasMouseTarget(m, target) {
			t.Errorf("details have no mouse target %v", target)
		}
	}
	for _, word := range strings.Fields(note) {
		if !strings.Contains(view, word) {
			t.Fatalf("wrapped note lost %q:\n%s", word, view)
		}
	}
	if strings.Contains(view, entry.Path) || strings.Contains(view, "…") {
		t.Fatalf("details expose or truncate the vCard path/note:\n%s", view)
	}
}

func TestContactDetailsLoadLegacyVCardTypes(t *testing.T) {
	dir := t.TempDir()
	book := config.Source{Path: dir, Type: "addressbook", DisplayName: "Legacy"}
	data := strings.Join([]string{
		"BEGIN:VCARD",
		"VERSION:2.1",
		"FN:Fixture",
		"EMAIL;INTERNET;HOME:fixture@example.test",
		"TEL;CELL;VOICE:+1-555-0100",
		"ADR;HOME:;;1 Main Street;London;;;UK",
		"END:VCARD",
		"",
	}, "\r\n")
	if err := os.WriteFile(filepath.Join(dir, "fixture.vcf"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	store := contact.NewStore(&config.Config{Sources: []config.Source{book}})
	contacts, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("loaded %d contacts, want 1", len(contacts))
	}
	m := &model{visible: contacts, contacts: contacts, styles: DefaultStyles(), mouse: &mouseState{}}
	view := ansi.Strip(m.contactDetailsView(60, 30, 0, 0))
	for _, want := range []string{
		"Email (internet, home)", "fixture@example.test",
		"Phone (cell, voice)", "+1-555-0100",
		"Address (home)", "1 Main Street, London, UK",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("legacy details do not contain %q:\n%s", want, view)
		}
	}
	if _, err := store.Save(contacts[0].Card, book, contacts[0].Path); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(contacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	var emailLine string
	for line := range strings.SplitSeq(string(saved), "\n") {
		if strings.HasPrefix(line, "EMAIL;") {
			emailLine = line
			break
		}
	}
	if !strings.Contains(emailLine, "TYPE=") || !strings.Contains(emailLine, "INTERNET") || !strings.Contains(emailLine, "HOME") {
		t.Fatalf("saved legacy email does not use explicit TYPE parameters: %q", emailLine)
	}
}

func TestDetailActionTargetsRejectInvalidValues(t *testing.T) {
	for _, test := range []struct {
		name   string
		action detailAction
		value  string
		want   string
		valid  bool
	}{
		{name: "email", action: detailActionMail, value: " ada@example.test ", want: "mailto:ada@example.test", valid: true},
		{name: "prefixed email", action: detailActionMail, value: "mailto:ada@example.test"},
		{name: "web page", action: detailActionOpen, value: "https://example.test/path", want: "https://example.test/path", valid: true},
		{name: "uppercase HTTPS", action: detailActionOpen, value: "HTTPS://example.test", want: "HTTPS://example.test", valid: true},
		{name: "bare host", action: detailActionOpen, value: "example.test"},
		{name: "file", action: detailActionOpen, value: "file:///tmp/contact"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := detailActionTarget(test.action, test.value)
			if test.valid && (err != nil || got != test.want) {
				t.Fatalf("detailActionTarget() = %q, %v; want %q", got, err, test.want)
			}
			if !test.valid && err == nil {
				t.Fatalf("detailActionTarget() accepted %q as %q", test.value, got)
			}
		})
	}
	m := &model{styles: DefaultStyles()}
	for _, key := range []string{vcard.FieldEmail, vcard.FieldURL} {
		line, buttons := m.detailFieldActionLine(editorRow{key: key, value: " \t "}, 60)
		if line != "" || len(buttons) != 0 {
			t.Errorf("empty %s field has actions: %q, %#v", key, line, buttons)
		}
	}
}

func TestWorkspacePaneNavigationAndFormOrigin(t *testing.T) {
	entry := contact.Contact{Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Ada"}}}}
	m := &model{mode: modeList, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.focusedPane != paneDetails || m.mode != modeList {
		t.Fatalf("Enter from Contacts: pane=%v mode=%v", m.focusedPane, m.mode)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.mode != modeForm || m.formOrigin != paneDetails {
		t.Fatalf("e from Details: mode=%v origin=%v", m.mode, m.formOrigin)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeList || m.focusedPane != paneDetails {
		t.Fatalf("cancelled form: mode=%v pane=%v", m.mode, m.focusedPane)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.focusedPane != paneContacts {
		t.Fatalf("h from Details focused pane %v", m.focusedPane)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedPane != paneDetails {
		t.Fatalf("Tab focused pane %v", m.focusedPane)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.mode != modeForm || m.form.editing != nil || m.formOrigin != paneDetails {
		t.Fatalf("n from Details: mode=%v editing=%#v origin=%v", m.mode, m.form.editing, m.formOrigin)
	}
}
func TestMouseWheelScrollsPaneUnderPointer(t *testing.T) {
	entry := contact.Contact{Card: vcard.Card{
		vcard.FieldFormattedName: []*vcard.Field{{Value: "Ada"}},
		vcard.FieldNote:          []*vcard.Field{{Value: strings.Repeat("long note ", 100)}},
	}}
	m := &model{
		mode:     modeList,
		width:    100,
		height:   20,
		contacts: []contact.Contact{entry},
		visible:  []contact.Contact{entry},
		selected: map[string]bool{},
		mouse:    &mouseState{},
	}
	_ = m.View()

	_, _ = m.updateMouse(tea.MouseEvent{X: 80, Y: 10, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.focusedPane != paneDetails || m.detailScroll != 1 {
		t.Fatalf("wheel over Details: pane=%v scroll=%d", m.focusedPane, m.detailScroll)
	}
}

func TestContactActionButtonsFollowSelectionAndDestinations(t *testing.T) {
	one := config.Source{Path: "/one", Type: "addressbook", DisplayName: "One"}
	two := config.Source{Path: "/two", Type: "addressbook", DisplayName: "Two"}
	contacts := []contact.Contact{
		{Path: "/one/a.vcf", Book: one, Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Ada"}}}},
		{Path: "/one/b.vcf", Book: one, Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Bob"}}}},
	}
	m := &model{mode: modeList, width: 100, height: 30, books: []config.Source{one, two}, contacts: contacts, visible: contacts, selected: map[string]bool{contacts[0].Path: true}, mouse: &mouseState{}}
	_ = m.View()
	if !hasMouseTarget(m, mouseMoveAction) || !hasMouseTarget(m, mouseCopyAction) || hasMouseTarget(m, mouseMergeAction) {
		t.Fatalf("one-selection actions = %#v", m.mouse.hits)
	}

	m.selected[contacts[1].Path] = true
	_ = m.View()
	if !hasMouseTarget(m, mouseMergeAction) {
		t.Fatal("Merge button is hidden for two selected contacts")
	}
}

func TestDetailActionFeedback(t *testing.T) {
	card := vcard.Card{
		vcard.FieldFormattedName: []*vcard.Field{{Value: "Ada"}},
		vcard.FieldURL:           []*vcard.Field{{Value: "https://example.test"}},
	}
	entry := contact.Contact{Path: "/contacts/ada.vcf", Card: card}
	m := &model{
		visible:              []contact.Contact{entry},
		contacts:             []contact.Contact{entry},
		styles:               DefaultStyles(),
		mouse:                &mouseState{},
		detailActionStatusID: 7,
		detailStatus: detailActionStatus{
			action: detailActionCopy,
			key:    detailPathKey,
			value:  entry.Path,
		},
	}

	updated, cmd := m.Update(detailActionResultMsg{id: 7})
	m = updated.(*model)
	if cmd == nil || !strings.Contains(m.contactDetailsView(60, 30, 0, 0), " Copied") {
		t.Fatal("successful copy did not show Copied or schedule expiry")
	}
	updated, _ = m.Update(detailActionExpiredMsg{id: 7})
	m = updated.(*model)
	if strings.Contains(m.contactDetailsView(60, 30, 0, 0), " Copied") {
		t.Fatal("Copied label did not expire")
	}

	m.detailActionStatusID = 8
	m.detailStatus = detailActionStatus{action: detailActionOpen, key: vcard.FieldURL, value: "https://example.test"}
	updated, _ = m.Update(detailActionResultMsg{id: 8, err: errors.New("no handler")})
	m = updated.(*model)
	if !strings.Contains(m.contactDetailsView(60, 30, 0, 0), "󰖟 Open Failed") {
		t.Fatal("failed open did not persist on its button")
	}
}

func TestEditDeleteCancelPreservesUnsavedContact(t *testing.T) {
	entry := contact.Contact{Path: "/contacts/ada.vcf", Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Persisted"}}}}
	m := &model{mode: modeList, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}}
	m.startForm(&m.visible[0], nil)
	m.form.card.SetValue(vcard.FieldFormattedName, "Unsaved")
	for i, row := range m.editorRows() {
		if row.key == "form-delete" {
			m.form.cursor = i
			break
		}
	}
	_, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeConfirm || m.dialogFocus != dialogFocusCancel {
		t.Fatalf("Delete action: mode=%v focus=%v", m.mode, m.dialogFocus)
	}
	_, _ = m.updateConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeForm || m.form.card.Value(vcard.FieldFormattedName) != "Unsaved" {
		t.Fatalf("cancelled deletion lost form: mode=%v card=%#v", m.mode, m.form.card)
	}
}
func TestSaveSelectsSavedContact(t *testing.T) {
	book := config.Source{Path: t.TempDir(), Type: "addressbook", DisplayName: "Personal"}
	cfg := &config.Config{Sources: []config.Source{book}}
	m := &model{
		store:    contact.NewStore(cfg),
		cfg:      cfg,
		books:    []config.Source{book},
		mode:     modeList,
		selected: map[string]bool{},
	}
	m.startForm(nil, nil)
	m.form.card.SetValue(vcard.FieldFormattedName, "Ada")

	if cmd := m.saveForm(); cmd != nil {
		t.Fatal("normal save returned a command")
	}
	if m.mode != modeList || m.current() == nil || m.current().Name() != "Ada" {
		t.Fatalf("saved contact is not selected: mode=%v current=%#v", m.mode, m.current())
	}
}

func TestFailedAddEmailSaveStaysInForm(t *testing.T) {
	temp := t.TempDir()
	file := filepath.Join(temp, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	book := config.Source{Path: file, Type: "addressbook", DisplayName: "Broken"}
	cfg := &config.Config{Sources: []config.Source{book}}
	m := &model{store: contact.NewStore(cfg), cfg: cfg, books: []config.Source{book}, quitAfterSave: true, mode: modeList}
	m.startForm(nil, nil)
	m.form.card.SetValue(vcard.FieldFormattedName, "Ada")

	if cmd := m.saveForm(); cmd != nil || m.mode != modeForm || m.form.errMsg == "" {
		t.Fatalf("failed add-email save: command=%v mode=%v error=%q", cmd, m.mode, m.form.errMsg)
	}
}

func TestWorkspaceKeepsContactsVisibleBesideEditor(t *testing.T) {
	entry := contact.Contact{Card: vcard.Card{vcard.FieldFormattedName: []*vcard.Field{{Value: "Ada"}}}}
	m := &model{mode: modeList, width: 100, height: 30, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}, mouse: &mouseState{}}
	m.startForm(&m.visible[0], nil)
	for i, row := range m.editorRows() {
		if row.key == "form-delete" {
			m.form.cursor = i
			break
		}
	}

	view := ansi.Strip(m.View())
	for _, want := range []string{"Contacts", "Ada", "Edit contact", "Save", "Cancel", "Delete"} {
		if !strings.Contains(view, want) {
			t.Fatalf("workspace editor does not contain %q:\n%s", want, view)
		}
	}
}

func hasMouseTarget(m *model, target mouseTarget) bool {
	for _, hit := range m.mouse.hits {
		if hit.kind == target {
			return true
		}
	}
	return false
}

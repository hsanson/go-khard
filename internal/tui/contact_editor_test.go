package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/config"
	"github.com/hsanson/go-khard/internal/contact"
)

func TestEditorRowsCoverKhardTemplate(t *testing.T) {
	m := &model{
		books: []config.Source{{Path: "/tmp/contacts", Type: "addressbook", DisplayName: "Contacts"}},
		form:  formState{card: make(vcard.Card)},
	}
	rows := m.editorRows()
	want := []string{
		vcard.FieldFormattedName, vcard.FieldKind,
		"name-prefix", "name-first", "name-additional", "name-last", "name-suffix",
		vcard.FieldNickname, vcard.FieldAnniversary, vcard.FieldBirthday,
		vcard.FieldOrganization, vcard.FieldTitle, vcard.FieldRole,
		vcard.FieldTelephone, vcard.FieldEmail, vcard.FieldAddress,
		vcard.FieldCategories, vcard.FieldURL, "private-add", vcard.FieldNote, "addressbook",
	}
	for _, key := range want {
		found := false
		for _, row := range rows {
			if row.key == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("editor has no row for %s", key)
		}
	}
}

func TestEditHidesAddressbookAndAddShowsItFirst(t *testing.T) {
	books := []config.Source{
		{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"},
		{Path: "/tmp/two", Type: "addressbook", DisplayName: "Two"},
	}
	m := &model{books: books, form: formState{card: make(vcard.Card), editing: &contact.Contact{}}}
	for _, row := range m.editorRows() {
		if row.key == "addressbook" {
			t.Fatal("edit form allows changing addressbook")
		}
	}
	m.form = formState{}
	m.startForm(nil, nil)
	if m.mode != modeForm {
		t.Fatalf("startForm() mode = %v", m.mode)
	}
	rows := m.editorRows()
	if len(rows) < 2 || !rows[0].section || rows[1].key != "addressbook" {
		t.Fatalf("new contact rows do not start with addressbook: %#v", rows[:min(2, len(rows))])
	}
}

func TestShowUsesReadOnlyEditorAndEditShortcut(t *testing.T) {
	book := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	card.AddValue(vcard.FieldEmail, "ada@example.net")
	entry := contact.Contact{Card: card, Path: "/tmp/one/ada.vcf", Book: book}
	m := &model{books: []config.Source{book}}
	m.startShow(&entry)

	if m.mode != modeShow {
		t.Fatalf("startShow() mode = %v", m.mode)
	}
	foundBook := false
	for _, row := range m.editorRows() {
		if row.add {
			t.Fatalf("show view contains add row: %#v", row)
		}
		if row.key == "addressbook" {
			foundBook = true
		}
	}
	if !foundBook {
		t.Fatal("show view does not display addressbook")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.mode != modeForm || m.form.editing == nil {
		t.Fatalf("e did not switch show view to edit: mode=%v", m.mode)
	}
}

func TestActivePopupRoutesNavigationMessagesAndEscape(t *testing.T) {
	first, second := "", ""
	form := popup(huh.NewInput().Title("First").Value(&first), huh.NewInput().Title("Second").Value(&second))
	m := &model{mode: modeForm, form: formState{card: make(vcard.Card), activeForm: form}}
	before := form.GetFocusedField()
	_, _ = m.Update(huh.NextField())
	if form.GetFocusedField() == before {
		t.Fatal("next-field message was not routed to popup")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.form.activeForm != nil {
		t.Fatal("escape did not close popup")
	}
}

func TestDateValidation(t *testing.T) {
	for _, valid := range []string{"", "1815-12-10", "2000-02-29"} {
		if err := optionalDate(valid); err != nil {
			t.Errorf("optionalDate(%q) = %v", valid, err)
		}
	}
	for _, invalid := range []string{"10-12-1815", "1815-02-30", "1815-2-3"} {
		if err := optionalDate(invalid); err == nil {
			t.Errorf("optionalDate(%q) succeeded", invalid)
		}
	}
}

func TestCompactVCardDatesAreFormattedForDisplay(t *testing.T) {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldBirthday, "20100119")
	card.SetValue(vcard.FieldAnniversary, "2005-12-03")
	if got := scalarValue(card, vcard.FieldBirthday); got != "2010-01-19" {
		t.Fatalf("birthday display = %q", got)
	}
	if got := scalarValue(card, vcard.FieldAnniversary); got != "2005-12-03" {
		t.Fatalf("anniversary display = %q", got)
	}
	for _, unchanged := range []string{"--0119", "text=unknown", "invalid"} {
		if got := displayDate(unchanged); got != unchanged {
			t.Errorf("displayDate(%q) = %q", unchanged, got)
		}
	}
}

func TestEmailValidation(t *testing.T) {
	for _, valid := range []string{"ada@example.net", "user+tag@example.co.jp"} {
		if err := validEmail(valid); err != nil {
			t.Errorf("validEmail(%q) = %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "not-an-email", "Ada <ada@example.net>", "ada@"} {
		if err := validEmail(invalid); err == nil {
			t.Errorf("validEmail(%q) succeeded", invalid)
		}
	}
}

func TestNoteContinuationLinesAreIndented(t *testing.T) {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldNote, "first line\nsecond line")
	m := &model{width: 80, height: 200, mode: modeForm, form: formState{card: card}}
	view := m.formView()
	indent := strings.Repeat(" ", 24)
	if !strings.Contains(view, "\n"+indent+fieldValueStyle.Render("second line")) {
		t.Fatalf("note continuation is not aligned with value column:\n%s", view)
	}
}

func TestEditorAppliesStructuredAndTypedValues(t *testing.T) {
	m := &model{form: formState{card: make(vcard.Card)}}
	m.form.activeRow = editorRow{key: vcard.FieldEmail, add: true}
	m.form.tmp = []string{"work", "ada@example.net"}
	m.applyEditorPopup()
	if got := m.form.card[vcard.FieldEmail][0]; got.Value != "ada@example.net" || got.Params.Get(vcard.ParamType) != "work" {
		t.Fatalf("email = %#v", got)
	}

	m.form.activeRow = editorRow{key: vcard.FieldOrganization, add: true}
	m.form.tmp = []string{"Analytical Engines", "Research"}
	m.applyEditorPopup()
	if got := m.form.card.Value(vcard.FieldOrganization); got != "Analytical Engines;Research" {
		t.Fatalf("organization = %q", got)
	}

	m.form.activeRow = editorRow{key: vcard.FieldAddress, add: true}
	m.form.tmp = []string{"home", "", "Apartment 1", "First Street", "London", "London", "N1", "UK"}
	m.applyEditorPopup()
	address := m.form.card[vcard.FieldAddress][0]
	if address.Value != ";Apartment 1;First Street;London;London;N1;UK" || address.Params.Get(vcard.ParamType) != "home" {
		t.Fatalf("address = %#v", address)
	}

	m.form.activeRow = editorRow{key: "private-add", add: true}
	m.form.tmp = []string{"Jabber", "ada@example.net"}
	m.applyEditorPopup()
	if got := m.form.card.Value("X-JABBER"); got != "ada@example.net" {
		t.Fatalf("private property = %q", got)
	}
}

func TestFormattedNameFromComponents(t *testing.T) {
	card := make(vcard.Card)
	card.SetName(&vcard.Name{HonorificPrefix: "Countess", GivenName: "Ada", FamilyName: "Lovelace"})
	if got := formattedNameFromCard(card); got != "Countess Ada Lovelace" {
		t.Fatalf("formatted name = %q", got)
	}
}

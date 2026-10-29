package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

func TestEnterFocusesDetailsAndEditorHasDeleteWithoutPath(t *testing.T) {
	book := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	card.AddValue(vcard.FieldEmail, "ada@example.net")
	entry := contact.Contact{Card: card, Path: "/tmp/one/ada.vcf", Book: book}
	m := &model{mode: modeList, books: []config.Source{book}, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeList || m.focusedPane != paneDetails {
		t.Fatalf("enter from Contacts: mode=%v pane=%v", m.mode, m.focusedPane)
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeForm || m.form.editing == nil {
		t.Fatalf("enter from Details did not edit: mode=%v editing=%#v", m.mode, m.form.editing)
	}

	foundDelete := false
	for _, row := range m.editorRows() {
		if row.key == "addressbook" {
			t.Fatal("existing contact editor allows changing addressbook")
		}
		if row.key == "file-path" || strings.Contains(row.value, entry.Path) {
			t.Fatal("edit form still displays the vCard path")
		}
		if row.key == "form-delete" {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Fatal("edit form has no Delete action")
	}
}

func TestMergeConflictsAreResolvedSequentiallyBeforeReview(t *testing.T) {
	book := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	first, second := make(vcard.Card), make(vcard.Card)
	first.SetValue(vcard.FieldFormattedName, "Alex Sanson")
	first.SetName(&vcard.Name{GivenName: "Alex", FamilyName: "Sanson"})
	first.SetValue(vcard.FieldBirthday, "2000-01-01")
	first.AddValue(vcard.FieldEmail, "shared@example.net")
	second.SetValue(vcard.FieldFormattedName, "Alejandro Sanson")
	second.SetName(&vcard.Name{GivenName: "Alejandro", FamilyName: "Sanson"})
	second.SetValue(vcard.FieldBirthday, "2001-02-03")
	second.AddValue(vcard.FieldEmail, "shared@example.net")
	second.AddValue(vcard.FieldEmail, "other@example.net")
	contacts := []contact.Contact{
		{Card: first, Path: "/tmp/one/first.vcf", Book: book},
		{Card: second, Path: "/tmp/one/second.vcf", Book: book},
	}
	m := &model{
		books:    []config.Source{book},
		contacts: contacts,
		visible:  contacts,
		selected: map[string]bool{contacts[0].Path: true, contacts[1].Path: true},
	}
	_ = m.startMerge()
	if m.mode != modeConflict || m.conflictForm == nil || len(m.conflicts) != 3 {
		t.Fatalf("merge did not open sequential conflicts: mode=%v conflicts=%#v", m.mode, m.conflicts)
	}
	for m.mode == modeConflict {
		key := m.conflicts[m.conflictCursor]
		values := m.conflictValues[key]
		m.conflictValue = values[len(values)-1]
		_, _ = m.applyConflict()
	}
	if m.mode != modeForm || len(m.form.merged) != 2 {
		t.Fatalf("merge did not reach review form: mode=%v", m.mode)
	}
	if got := m.form.card.Value(vcard.FieldFormattedName); got != "Alejandro Sanson" {
		t.Fatalf("selected formatted name = %q", got)
	}
	if got := m.form.card.Name().GivenName; got != "Alejandro" {
		t.Fatalf("selected first name = %q", got)
	}
	if got := len(m.form.card.Values(vcard.FieldEmail)); got != 2 {
		t.Fatalf("merged email count = %d", got)
	}
	foundBook := false
	for _, row := range m.editorRows() {
		if row.key == "addressbook" {
			foundBook = true
		}
	}
	if !foundBook {
		t.Fatal("merge review does not expose target addressbook")
	}
}

func TestActivePopupRoutesNavigationMessagesAndEscape(t *testing.T) {
	first, second := "", ""
	form := popup(nil, huh.NewInput().Title("First").Value(&first), huh.NewInput().Title("Second").Value(&second))
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
	if !strings.Contains(view, "\n"+indent+m.styles.Value.Render("second line")) {
		t.Fatalf("note continuation is not aligned with value column:\n%s", view)
	}
}

func TestNoteCtrlEnterInsertsNewline(t *testing.T) {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldNote, "first line")
	m := &model{mode: modeForm, form: formState{card: card}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldNote {
			m.form.cursor = i
			break
		}
	}
	teaModel := runHuhNavigation(m, m.openEditorPopup(rows))
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = teaModel.(*model)
	if got := m.form.tmp[0]; got != "first line\n" {
		t.Fatalf("note after Ctrl-Enter = %q", got)
	}
}

func TestEditorAppliesStructuredAndTypedValues(t *testing.T) {
	m := &model{form: formState{card: make(vcard.Card)}}
	m.form.activeRow = editorRow{key: vcard.FieldEmail, add: true}
	m.form.tmp = []string{"ada@example.net"}
	m.form.tmpTypes = []string{"work", "internet"}
	m.applyEditorPopup()
	if got := m.form.card[vcard.FieldEmail][0]; got.Value != "ada@example.net" ||
		len(got.Params.Types()) != 2 || got.Params.Types()[0] != "work" || got.Params.Types()[1] != "internet" {
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

func TestTypedFieldEditsPreserveUnrelatedParameters(t *testing.T) {
	card := make(vcard.Card)
	card.Add(vcard.FieldEmail, &vcard.Field{
		Value: "old@example.test",
		Params: vcard.Params{
			vcard.ParamType:      []string{"home"},
			vcard.ParamPreferred: []string{"1"},
			vcard.ParamPID:       []string{"1.1"},
		},
		Group: "item1",
	})
	card.Add(vcard.FieldAddress, &vcard.Field{
		Value: ";;Old Street;Old City;Old Region;00000;Old Country",
		Params: vcard.Params{
			vcard.ParamType: []string{"home"},
			"X-META":        []string{"keep"},
		},
		Group: "item2",
	})
	m := &model{form: formState{
		card:      card,
		activeRow: editorRow{key: vcard.FieldEmail},
		tmp:       []string{"new@example.test"},
		tmpTypes:  []string{"work"},
	}}

	m.applyEditorPopup()

	field := m.form.card[vcard.FieldEmail][0]
	if field.Value != "new@example.test" || field.Group != "item1" || !reflect.DeepEqual(field.Params.Types(), []string{"work"}) {
		t.Fatalf("edited email = %#v", field)
	}
	if field.Params.Get(vcard.ParamPreferred) != "1" || field.Params.Get(vcard.ParamPID) != "1.1" {
		t.Fatalf("edited email lost unrelated parameters: %#v", field.Params)
	}

	m.form.activeRow = editorRow{key: vcard.FieldAddress}
	m.form.tmp = []string{"work", "", "", "New Street", "New City", "New Region", "11111", "New Country"}
	m.form.tmpTypes = nil
	m.applyEditorPopup()

	field = m.form.card[vcard.FieldAddress][0]
	if field.Value != ";;New Street;New City;New Region;11111;New Country" ||
		field.Group != "item2" || !reflect.DeepEqual(field.Params.Types(), []string{"work"}) {
		t.Fatalf("edited address = %#v", field)
	}
	if field.Params.Get("X-META") != "keep" {
		t.Fatalf("edited address lost unrelated parameters: %#v", field.Params)
	}
}

func TestTypedPopupPreservesMultipleAndUnknownTypes(t *testing.T) {
	card := make(vcard.Card)
	card.Add(vcard.FieldTelephone, &vcard.Field{
		Value:  "+81-3-1234-5678",
		Params: vcard.Params{vcard.ParamType: []string{"cell", "satellite"}},
	})
	m := &model{cfg: config.Default(), form: formState{card: card}}
	_ = m.typedPopup(editorRow{key: vcard.FieldTelephone, index: 0})
	if len(m.form.tmpTypes) != 2 || m.form.tmpTypes[0] != "cell" || m.form.tmpTypes[1] != "satellite" {
		t.Fatalf("popup types = %#v", m.form.tmpTypes)
	}
}

func TestFormattedNameFromComponents(t *testing.T) {
	card := make(vcard.Card)
	card.SetName(&vcard.Name{HonorificPrefix: "Countess", GivenName: "Ada", FamilyName: "Lovelace"})
	if got := formattedNameFromCard(card); got != "Countess Ada Lovelace" {
		t.Fatalf("formatted name = %q", got)
	}
}

func TestEditorDialogTraversesFieldsAndActions(t *testing.T) {
	first, second := "", ""
	form := popup(nil,
		huh.NewInput().Title("First").Value(&first),
		huh.NewInput().Title("Second").Value(&second),
	)
	m := &model{mode: modeForm, form: formState{
		card: make(vcard.Card), activeForm: form, activeRow: editorRow{key: vcard.FieldOrganization},
		dialogFields: 2,
	}}
	before := form.GetFocusedField()
	_, _ = m.updateActiveEditorForm(tea.KeyMsg{Type: tea.KeyDown})
	if form.GetFocusedField() == before || m.form.dialogField != 1 {
		t.Fatal("Down did not focus the next field")
	}
	_, _ = m.updateActiveEditorForm(tea.KeyMsg{Type: tea.KeyDown})
	if m.form.dialogFocus != dialogFocusPrimary {
		t.Fatalf("Down from final field focus = %v", m.form.dialogFocus)
	}
	_, _ = m.updateActiveEditorForm(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.form.dialogFocus != dialogFocusControl || m.form.dialogField != 1 {
		t.Fatalf("Shift-Tab from Apply: focus=%v field=%d", m.form.dialogFocus, m.form.dialogField)
	}
}
func TestEnterAdvancesToNextEditorField(t *testing.T) {
	m := &model{cfg: config.Default(), mode: modeForm, form: formState{card: make(vcard.Card)}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldOrganization && row.add {
			m.form.cursor = i
			break
		}
	}
	teaModel := runHuhNavigation(m, m.openEditorPopup(rows))
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Analytical Engines")})
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyEnter})
	m = teaModel.(*model)

	if m.form.dialogField != 1 || m.form.dialogFocus != dialogFocusControl {
		t.Fatalf("Enter from first field: field=%d focus=%v", m.form.dialogField, m.form.dialogFocus)
	}
}

func TestJKChangesSelectAndMultiSelectOptions(t *testing.T) {
	m := &model{cfg: config.Default(), mode: modeForm, width: 100, form: formState{card: make(vcard.Card)}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldKind {
			m.form.cursor = i
			break
		}
	}
	teaModel := runHuhNavigation(m, m.openEditorPopup(rows))
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = teaModel.(*model)
	if got := m.form.tmp[0]; got != "individual" {
		t.Fatalf("Kind after j = %q", got)
	}
	m.cancelEditorDialog()

	rows = m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldTelephone && row.add {
			m.form.cursor = i
			break
		}
	}
	teaModel = runHuhNavigation(m, m.openEditorPopup(rows))
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeySpace})
	m = teaModel.(*model)
	if !containsString(m.form.tmpTypes, "home") {
		t.Fatalf("Phone Types after j/space = %#v", m.form.tmpTypes)
	}
}

func TestEnterOnFinalEditorFieldFocusesApply(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		first, last   string
		advanceFields int
	}{
		{name: "organization", key: vcard.FieldOrganization, first: "Analytical Engines", last: "Research", advanceFields: 1},
		{name: "phone", key: vcard.FieldTelephone, last: "+44 1234 567890", advanceFields: 1},
		{name: "email", key: vcard.FieldEmail, last: "ada@example.test", advanceFields: 1},
		{name: "postal address", key: vcard.FieldAddress, last: "UK", advanceFields: 7},
		{name: "private property", key: "private-add", first: "Jabber", last: "ada@example.test", advanceFields: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &model{cfg: config.Default(), mode: modeForm, form: formState{card: make(vcard.Card)}}
			rows := m.editorRows()
			for i, row := range rows {
				if row.key == test.key && row.add {
					m.form.cursor = i
					break
				}
			}
			teaModel := runHuhNavigation(m, m.openEditorPopup(rows))
			if test.first != "" {
				teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(test.first)})
			}
			for range test.advanceFields {
				teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyEnter})
			}
			teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(test.last)})
			m = teaModel.(*model)

			teaModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = teaModel.(*model)
			if m.form.dialogFocus != dialogFocusPrimary || m.form.activeForm == nil ||
				strings.TrimSpace(m.form.activeForm.View()) == "" {
				t.Fatalf("final Enter: focus=%v form=%v", m.form.dialogFocus, m.form.activeForm)
			}

			teaModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = teaModel.(*model)
			if m.form.activeForm != nil {
				t.Fatal("Enter on Apply left the field dialog open")
			}
		})
	}
}
func TestInvalidFinalEditorFieldKeepsFocus(t *testing.T) {
	m := &model{cfg: config.Default(), mode: modeForm, form: formState{card: make(vcard.Card)}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldEmail && row.add {
			m.form.cursor = i
			break
		}
	}
	teaModel := runHuhNavigation(m, m.openEditorPopup(rows))
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyEnter})
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("invalid")})
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyEnter})
	m = teaModel.(*model)

	if m.form.dialogFocus != dialogFocusControl || m.form.dialogField != 1 ||
		m.form.activeForm.GetFocusedField().Error() == nil {
		t.Fatalf("invalid final field: focus=%v field=%d error=%v",
			m.form.dialogFocus, m.form.dialogField, m.form.activeForm.GetFocusedField().Error())
	}
}
func TestCompletedEditorPopupDoesNotRenderEmptyDialog(t *testing.T) {
	m := &model{mode: modeForm, form: formState{card: make(vcard.Card)}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldFormattedName {
			m.form.cursor = i
			break
		}
	}
	teaModel := runHuhNavigation(m, m.openEditorPopup(rows))
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Ada Lovelace")})
	teaModel = updateAndRunHuhNavigation(teaModel, huh.NextField())
	m = teaModel.(*model)

	if m.form.activeForm != nil {
		t.Fatalf("completed form left an empty dialog open:\n%s", m.formView())
	}
	if got := m.form.card.Value(vcard.FieldFormattedName); got != "Ada Lovelace" {
		t.Fatalf("completed form value = %q", got)
	}
}

func TestDeleteActionRemovesExistingItemOnly(t *testing.T) {
	card := make(vcard.Card)
	card.AddValue(vcard.FieldEmail, "ada@example.net")
	m := &model{cfg: config.Default(), mode: modeForm, form: formState{card: card}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldEmail && !row.add {
			m.form.cursor = i
			_ = m.openEditorPopup(rows)
			break
		}
	}
	if !m.editorDialogHasDelete() {
		t.Fatal("existing email dialog has no Delete action")
	}
	m.form.dialogFocus = dialogFocusDelete
	_, _ = m.activateEditorDialog()
	if len(m.form.card[vcard.FieldEmail]) != 0 {
		t.Fatalf("email was not removed: %#v", m.form.card[vcard.FieldEmail])
	}

	rows = m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldEmail && row.add {
			m.form.cursor = i
			_ = m.openEditorPopup(rows)
			break
		}
	}
	if m.editorDialogHasDelete() {
		t.Fatal("new email dialog exposes Delete")
	}
}

func TestContactFormScrollsToFocusedActions(t *testing.T) {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	m := &model{width: 80, height: 10, mode: modeForm, form: formState{card: card}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == "form-cancel" {
			m.form.cursor = i
			break
		}
	}
	view := m.formBaseView()
	if !strings.Contains(view, "Save") || !strings.Contains(view, "Cancel") {
		t.Fatalf("focused form actions are outside the viewport:\n%s", view)
	}
}
func TestContactFormUsesResponsiveActionRows(t *testing.T) {
	for _, test := range []struct {
		width    int
		sameLine bool
	}{
		{width: 26, sameLine: true},
		{width: 24, sameLine: false},
	} {
		card := make(vcard.Card)
		card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
		entry := contact.Contact{Card: card}
		m := &model{
			width: test.width, height: 10, mode: modeForm, mouse: &mouseState{}, styles: DefaultStyles(),
			form: formState{card: card, editing: &entry},
		}
		for i, row := range m.editorRows() {
			if row.key == "form-delete" {
				m.form.cursor = i
				break
			}
		}
		lines := strings.Split(ansi.Strip(m.formBaseView()), "\n")
		saveLine, deleteLine := -1, -1
		for i, line := range lines {
			if strings.Contains(line, "Save") && strings.Contains(line, "Cancel") {
				saveLine = i
			}
			if strings.Contains(line, "Delete") {
				deleteLine = i
			}
		}
		if saveLine < 0 || deleteLine < 0 || (saveLine == deleteLine) != test.sameLine {
			t.Fatalf("width %d action rows:\n%s", test.width, strings.Join(lines, "\n"))
		}
		for _, hit := range m.mouse.hits {
			if hit.kind == mouseEditorRow && hit.index == m.form.cursor && hit.rect.y != deleteLine {
				t.Fatalf("width %d Delete mouse row = %d, rendered row = %d", test.width, hit.rect.y, deleteLine)
			}
		}
	}
}

func TestFieldDialogOverlaysContactForm(t *testing.T) {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	m := &model{width: 80, height: 30, mode: modeForm, form: formState{card: card}}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldFormattedName {
			m.form.cursor = i
			break
		}
	}
	_ = m.openEditorPopup(rows)
	view := m.formView()
	if !strings.Contains(view, "Roles") || !strings.Contains(view, "Apply") {
		t.Fatalf("overlay does not retain the contact form and actions:\n%s", view)
	}
}

func TestShortFieldDialogKeepsFocusedFieldsAndActionsVisible(t *testing.T) {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Fixture")
	card.Add(vcard.FieldAddress, &vcard.Field{
		Value:  ";;1 Main Street;London;Region;00000;UK",
		Params: vcard.Params{vcard.ParamType: []string{"home"}},
	})
	m := &model{
		cfg: config.Default(), width: 80, height: 12, mode: modeForm,
		styles: DefaultStyles(), mouse: &mouseState{}, form: formState{card: card},
	}
	rows := m.editorRows()
	for i, row := range rows {
		if row.key == vcard.FieldAddress && !row.add {
			m.form.cursor = i
			break
		}
	}
	_ = m.openEditorPopup(rows)

	for index, title := range []string{"Type", "Box", "Extended", "Street", "Code", "City", "Region", "Country"} {
		m.mouse.reset()
		view := ansi.Strip(m.formView())
		if !strings.Contains(view, title) {
			t.Fatalf("focused field %q is outside the short popup:\n%s", title, view)
		}
		for _, action := range []string{"Apply", "Cancel", "Delete"} {
			if !strings.Contains(view, action) {
				t.Fatalf("%s is outside the short popup while %q is focused:\n%s", action, title, view)
			}
		}
		for _, hit := range m.mouse.hits {
			if hit.kind == mouseDialogAction && (hit.rect.y < 0 || hit.rect.y >= lipgloss.Height(view)) {
				t.Fatalf("action mouse target is outside the short popup: %#v\n%s", hit, view)
			}
		}
		if index < m.form.dialogFields-1 {
			var updated tea.Model
			if index == 0 {
				updated, _ = m.updateMouse(tea.MouseEvent{Button: tea.MouseButtonWheelDown})
			} else {
				updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
			}
			m = updated.(*model)
		}
	}
}

func TestDialogsUseSixtyPercentOfTerminalWidth(t *testing.T) {
	m := &model{width: 100, height: 40, styles: DefaultStyles()}
	want := 60
	dialogs := []struct {
		name string
		view string
	}{
		{name: "centered", view: m.centerDialog("Confirm", nil)},
		{name: "form overlay", view: m.overlayDialog(strings.Repeat("x", m.width), "Field", nil)},
		{name: "help", view: m.renderHelpOverlay(m.mainHeight())},
	}
	for _, dialog := range dialogs {
		if got := renderedDialogWidth(dialog.view); got != want {
			t.Errorf("%s dialog width = %d, want %d", dialog.name, got, want)
		}
	}
}

func renderedDialogWidth(view string) int {
	for line := range strings.SplitSeq(ansi.Strip(view), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "╭") {
			return lipgloss.Width(line)
		}
	}
	return 0
}

func TestDeleteActionUsesResponsiveRowsAndMouseTargets(t *testing.T) {
	for _, test := range []struct {
		name     string
		width    int
		sameLine bool
	}{
		{name: "fits one row", width: 60, sameLine: true},
		{name: "wraps when required", width: 50, sameLine: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			card := make(vcard.Card)
			card.AddValue(vcard.FieldEmail, "ada@example.test")
			m := &model{
				cfg: config.Default(), mode: modeForm, width: test.width, height: 40,
				mouse: &mouseState{}, styles: DefaultStyles(), form: formState{card: card},
			}
			rows := m.editorRows()
			for i, row := range rows {
				if row.key == vcard.FieldEmail && !row.add {
					m.form.cursor = i
					_ = m.openEditorPopup(rows)
					break
				}
			}
			m.form.dialogFocus = dialogFocusDelete
			view := strings.Split(ansi.Strip(m.formView()), "\n")
			applyLine, deleteLine := -1, -1
			for i, line := range view {
				if strings.Contains(line, "Apply") && strings.Contains(line, "Cancel") {
					applyLine = i
				}
				if strings.Contains(line, "Delete") {
					deleteLine = i
				}
			}
			if applyLine < 0 || deleteLine < 0 || (applyLine == deleteLine) != test.sameLine {
				t.Fatalf("action rows do not match width %d:\n%s", test.width, strings.Join(view, "\n"))
			}
			for _, hit := range m.mouse.hits {
				if hit.kind == mouseDialogAction && hit.focus == dialogFocusDelete && hit.rect.y != deleteLine {
					t.Fatalf("Delete mouse row = %d, rendered row = %d", hit.rect.y, deleteLine)
				}
			}
		})
	}
}

func updateAndRunHuhNavigation(teaModel tea.Model, msg tea.Msg) tea.Model {
	updated, cmd := teaModel.Update(msg)
	return runHuhNavigation(updated, cmd)
}

func runHuhNavigation(teaModel tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return teaModel
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			teaModel = runHuhNavigation(teaModel, child)
		}
		return teaModel
	}
	typeOf := reflect.TypeOf(msg)
	if typeOf == nil || typeOf.PkgPath() != "github.com/charmbracelet/huh" {
		return teaModel
	}
	switch typeOf.Name() {
	case "nextFieldMsg", "prevFieldMsg", "nextGroupMsg", "prevGroupMsg":
		updated, next := teaModel.Update(msg)
		return runHuhNavigation(updated, next)
	default:
		return teaModel
	}
}

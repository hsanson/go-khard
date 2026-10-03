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

func TestEnterOpensEditorAndPathIsNotSelectable(t *testing.T) {
	book := config.Source{Path: "/tmp/one", Type: "addressbook", DisplayName: "One"}
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	card.AddValue(vcard.FieldEmail, "ada@example.net")
	entry := contact.Contact{Card: card, Path: "/tmp/one/ada.vcf", Book: book}
	m := &model{mode: modeList, books: []config.Source{book}, contacts: []contact.Contact{entry}, visible: []contact.Contact{entry}, selected: map[string]bool{}}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeForm || m.form.editing == nil {
		t.Fatalf("enter did not open contact editor: mode=%v editing=%#v", m.mode, m.form.editing)
	}

	rows := m.editorRows()
	pathIndex := -1
	for i, row := range rows {
		if row.key == "addressbook" {
			t.Fatal("existing contact editor allows changing addressbook")
		}
		if row.key == "file-path" && row.value == entry.Path {
			pathIndex = i
			m.form.cursor = i
			if cmd := m.openEditorPopup(rows); cmd != nil {
				t.Fatal("file path row opened an editor")
			}
		}
	}
	if pathIndex < 0 {
		t.Fatal("edit view does not display vCard path")
	}
	m.form.cursor = pathIndex - 1
	m.moveEditorCursor(rows, 1)
	if rows[m.form.cursor].key == "file-path" || rows[m.form.cursor].section {
		t.Fatalf("navigation selected non-editable row: %#v", rows[m.form.cursor])
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
	if !strings.Contains(view, "\n"+indent+m.styles.FieldValue.Render("second line")) {
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

func TestEnterOnFinalEditorFieldApplies(t *testing.T) {
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
	teaModel = updateAndRunHuhNavigation(teaModel, tea.KeyMsg{Type: tea.KeyEnter})
	m = teaModel.(*model)
	if m.form.activeForm != nil {
		t.Fatal("Enter on the final field left an empty action dialog open")
	}
	if got := m.form.card.Value(vcard.FieldFormattedName); got != "Ada Lovelace" {
		t.Fatalf("formatted name after Enter = %q", got)
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

func TestDeleteActionReservesAnEmptyButtonSlot(t *testing.T) {
	_, hits := dialogWithActions("", []dialogAction{
		{label: "Apply", focus: dialogFocusPrimary},
		{label: "Cancel", focus: dialogFocusCancel},
		{label: "Delete", focus: dialogFocusDelete, destructive: true},
	}, dialogFocusControl, DefaultStyles())
	cancel, remove := hits[1].rect, hits[2].rect
	if gap := remove.x - (cancel.x + cancel.width); gap < remove.width+4 {
		t.Fatalf("Cancel/Delete gap = %d, want room for another button", gap)
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

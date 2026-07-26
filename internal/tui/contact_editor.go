package tui

import (
	"errors"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/emersion/go-vcard"
)

var sectionIcons = map[string]string{
	"identity": "󰀄", "nickname": "󰈉", "dates": "󰃭", "work": "󰃖", "title": "󰗴", "role": "󰓾", "phones": "󰏲",
	"emails": "󰇮", "addresses": "󰍎", "categories": "󰓹", "online": "󰖟", "private": "󰌾",
	"notes": "󰎞", "storage": "󰆼",
}

func (m *model) editorRows() []editorRow {
	card := m.form.card
	var rows []editorRow
	section := func(key, title string) {
		rows = append(rows, editorRow{section: true, label: sectionIcons[key] + "  " + title})
	}
	scalar := func(key, label string) {
		rows = append(rows, editorRow{key: key, label: label, value: scalarValue(card, key)})
	}
	repeat := func(key, label, addLabel string) {
		for i, f := range card[key] {
			rows = append(rows, editorRow{key: key, index: i, label: indexedLabel(label, f, i), value: displayField(key, f)})
		}
		if m.mode != modeShow {
			rows = append(rows, editorRow{key: key, label: "󰐕 Add " + addLabel, add: true})
		}
	}

	if m.mode == modeShow || m.form.editing == nil {
		section("storage", "Address book")
		book := "(no addressbooks)"
		if len(m.books) > 0 {
			book = m.books[m.form.book].Name()
		}
		rows = append(rows, editorRow{key: "addressbook", label: "Addressbook", value: book})
	}

	section("identity", "Identity")
	scalar(vcard.FieldFormattedName, "Formatted name")
	scalar(vcard.FieldKind, "Kind")
	scalar("name-prefix", "Prefix")
	scalar("name-first", "First name")
	scalar("name-additional", "Additional")
	scalar("name-last", "Last name")
	scalar("name-suffix", "Suffix")
	section("nickname", "Nicknames")
	repeat(vcard.FieldNickname, "Nickname", "nickname")

	section("dates", "Important dates")
	scalar(vcard.FieldAnniversary, "Anniversary")
	scalar(vcard.FieldBirthday, "Birthday")

	section("work", "Organisation")
	repeat(vcard.FieldOrganization, "Organisation", "organisation")
	section("title", "Titles")
	repeat(vcard.FieldTitle, "Title", "title")
	section("role", "Roles")
	repeat(vcard.FieldRole, "Role", "role")

	section("phones", "Phone numbers")
	repeat(vcard.FieldTelephone, "Phone", "phone")
	section("emails", "Email addresses")
	repeat(vcard.FieldEmail, "Email", "email")
	section("addresses", "Postal addresses")
	repeat(vcard.FieldAddress, "Address", "address")

	section("categories", "Categories")
	repeat(vcard.FieldCategories, "Category", "category")
	section("online", "Web pages")
	repeat(vcard.FieldURL, "Webpage", "webpage")

	section("private", "Private properties")
	for _, key := range privateKeys(card) {
		for i, f := range card[key] {
			rows = append(rows, editorRow{key: key, index: i, label: strings.TrimPrefix(key, "X-"), value: f.Value})
		}
	}
	if m.mode != modeShow {
		rows = append(rows, editorRow{key: "private-add", label: "󰐕 Add private property", add: true})
	}

	section("notes", "Notes")
	scalar(vcard.FieldNote, "Note")
	return rows
}

func (m *model) moveEditorCursor(rows []editorRow, delta int) {
	if len(rows) == 0 {
		return
	}
	for range rows {
		m.form.cursor = (m.form.cursor + delta + len(rows)) % len(rows)
		if !rows[m.form.cursor].section {
			return
		}
	}
}

func (m *model) updateActiveEditorForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
		m.form.activeForm = nil
		m.form.tmp = nil
		return m, nil
	}
	updated, cmd := m.form.activeForm.Update(msg)
	if form, ok := updated.(*huh.Form); ok {
		m.form.activeForm = form
	}
	switch m.form.activeForm.State {
	case huh.StateAborted:
		m.form.activeForm = nil
		m.form.tmp = nil
	case huh.StateCompleted:
		m.applyEditorPopup()
		m.form.activeForm = nil
		m.form.tmp = nil
	}
	return m, cmd
}

func (m *model) openEditorPopup(rows []editorRow) tea.Cmd {
	if len(rows) == 0 || m.form.cursor >= len(rows) {
		return nil
	}
	row := rows[m.form.cursor]
	if row.section {
		return nil
	}
	m.form.activeRow = row
	m.form.tmp = nil
	m.form.errMsg = ""
	m.form.activeForm = m.buildEditorPopup(row)
	if m.form.activeForm == nil {
		return nil
	}
	return m.form.activeForm.Init()
}

func (m *model) buildEditorPopup(row editorRow) *huh.Form {
	card := m.form.card
	if row.key == "addressbook" {
		options := make([]huh.Option[int], 0, len(m.books))
		for i, book := range m.books {
			options = append(options, huh.NewOption(book.Name(), i))
		}
		return popup(huh.NewSelect[int]().Title("Addressbook").Options(options...).Value(&m.form.book))
	}
	if row.key == "private-add" || strings.HasPrefix(row.key, "X-") {
		name, value := strings.TrimPrefix(row.key, "X-"), ""
		if !row.add {
			value = card[row.key][row.index].Value
		}
		m.form.tmp = []string{name, value}
		return popup(
			huh.NewInput().Title("Property name").Description("Letters, digits, and hyphens; stored with an X- prefix").Value(&m.form.tmp[0]).Validate(privateName),
			huh.NewInput().Title("Value").Value(&m.form.tmp[1]),
		)
	}
	if isNameKey(row.key) {
		m.form.tmp = []string{scalarValue(card, row.key)}
		return popup(huh.NewInput().Title(row.label).Description("Separate multiple values with commas").Value(&m.form.tmp[0]))
	}
	switch row.key {
	case vcard.FieldFormattedName:
		m.form.tmp = []string{card.Value(row.key)}
		return popup(huh.NewInput().Title(row.label).Value(&m.form.tmp[0]))
	case vcard.FieldKind:
		value := card.Value(row.key)
		m.form.tmp = []string{value}
		return popup(huh.NewSelect[string]().Title("Kind").Options(
			huh.NewOption("Unspecified", ""), huh.NewOption("Individual", "individual"),
			huh.NewOption("Group", "group"), huh.NewOption("Organisation", "org"),
			huh.NewOption("Location", "location"), huh.NewOption("Application", "application"),
			huh.NewOption("Device", "device"),
		).Value(&m.form.tmp[0]))
	case vcard.FieldBirthday, vcard.FieldAnniversary:
		m.form.tmp = []string{displayDate(card.Value(row.key))}
		return popup(huh.NewInput().Title(row.label + " (YYYY-MM-dd)").Value(&m.form.tmp[0]).Validate(optionalDate))
	case vcard.FieldTelephone, vcard.FieldEmail:
		return m.typedPopup(row)
	case vcard.FieldAddress:
		return m.addressPopup(row)
	case vcard.FieldOrganization:
		parts := []string{"", ""}
		if !row.add {
			parts = splitComponents(card[row.key][row.index].Value, 2)
		}
		m.form.tmp = parts
		return popup(huh.NewInput().Title("Company").Value(&m.form.tmp[0]), huh.NewInput().Title("Unit").Value(&m.form.tmp[1]))
	case vcard.FieldNote:
		m.form.tmp = []string{card.Value(row.key)}
		return popup(huh.NewText().Title("Note").Lines(10).Value(&m.form.tmp[0]))
	default:
		value := ""
		if !row.add {
			value = card[row.key][row.index].Value
		}
		m.form.tmp = []string{value}
		return popup(huh.NewInput().Title(strings.TrimPrefix(row.label, "󰐕 Add ")).Value(&m.form.tmp[0]))
	}
}

func (m *model) typedPopup(row editorRow) *huh.Form {
	field := &vcard.Field{}
	if !row.add {
		field = m.form.card[row.key][row.index]
	}
	allowed := m.cfg.PhoneTypes()
	if row.key == vcard.FieldEmail {
		allowed = m.cfg.EmailTypes()
	}
	current := strings.ToLower(field.Params.Get(vcard.ParamType))
	if current != "" && !containsString(allowed, current) {
		allowed = append(allowed, current)
	}
	m.form.tmp = []string{current, field.Value}
	options := []huh.Option[string]{huh.NewOption("Unspecified", "")}
	for _, value := range allowed {
		options = append(options, huh.NewOption(value, value))
	}
	valueInput := huh.NewInput().Title("Value").Value(&m.form.tmp[1]).Validate(required("value"))
	if row.key == vcard.FieldEmail {
		valueInput.Validate(validEmail)
	}
	return popup(
		huh.NewSelect[string]().Title("Type").Options(options...).Value(&m.form.tmp[0]),
		valueInput,
	)
}

func (m *model) addressPopup(row editorRow) *huh.Form {
	field := &vcard.Field{}
	if !row.add {
		field = m.form.card[row.key][row.index]
	}
	allowed := m.cfg.AddressTypes()
	current := strings.ToLower(field.Params.Get(vcard.ParamType))
	if current != "" && !containsString(allowed, current) {
		allowed = append(allowed, current)
	}
	parts := splitComponents(field.Value, 7)
	m.form.tmp = append([]string{current}, parts...)
	options := []huh.Option[string]{huh.NewOption("Unspecified", "")}
	for _, value := range allowed {
		options = append(options, huh.NewOption(value, value))
	}
	return popup(
		huh.NewSelect[string]().Title("Type").Options(options...).Value(&m.form.tmp[0]),
		huh.NewInput().Title("Box").Value(&m.form.tmp[1]),
		huh.NewInput().Title("Extended").Value(&m.form.tmp[2]),
		huh.NewInput().Title("Street").Value(&m.form.tmp[3]),
		huh.NewInput().Title("Code").Value(&m.form.tmp[6]),
		huh.NewInput().Title("City").Value(&m.form.tmp[4]),
		huh.NewInput().Title("Region").Value(&m.form.tmp[5]),
		huh.NewInput().Title("Country").Value(&m.form.tmp[7]),
	)
}

func (m *model) applyEditorPopup() {
	row := m.form.activeRow
	if row.key == "addressbook" {
		return
	}
	if row.key == "private-add" || strings.HasPrefix(row.key, "X-") {
		name := strings.ToUpper(strings.TrimSpace(m.form.tmp[0]))
		key := "X-" + strings.TrimPrefix(name, "X-")
		field := &vcard.Field{Value: strings.TrimSpace(m.form.tmp[1])}
		if row.add {
			if field.Value != "" {
				m.form.card.Add(key, field)
			}
		} else {
			oldKey := row.key
			m.form.card[oldKey] = removeField(m.form.card[oldKey], row.index)
			if len(m.form.card[oldKey]) == 0 {
				delete(m.form.card, oldKey)
			}
			if field.Value != "" {
				m.form.card.Add(key, field)
			}
		}
		return
	}
	if isNameKey(row.key) {
		setNameComponent(m.form.card, row.key, strings.TrimSpace(m.form.tmp[0]))
		return
	}
	if row.key == vcard.FieldFormattedName || row.key == vcard.FieldKind || row.key == vcard.FieldNote ||
		row.key == vcard.FieldBirthday || row.key == vcard.FieldAnniversary {
		setScalar(m.form.card, row.key, strings.TrimSpace(m.form.tmp[0]))
		return
	}
	field := &vcard.Field{}
	switch row.key {
	case vcard.FieldTelephone, vcard.FieldEmail:
		field.Value = strings.TrimSpace(m.form.tmp[1])
		setFieldType(field, m.form.tmp[0])
	case vcard.FieldAddress:
		if len(nonEmpty(m.form.tmp[1:]...)) == 0 {
			return
		}
		field.Value = strings.Join([]string{m.form.tmp[1], m.form.tmp[2], m.form.tmp[3], m.form.tmp[4], m.form.tmp[5], m.form.tmp[6], m.form.tmp[7]}, ";")
		setFieldType(field, m.form.tmp[0])
	case vcard.FieldOrganization:
		if strings.TrimSpace(m.form.tmp[0]) == "" && strings.TrimSpace(m.form.tmp[1]) == "" {
			return
		}
		field.Value = strings.Join([]string{strings.TrimSpace(m.form.tmp[0]), strings.TrimSpace(m.form.tmp[1])}, ";")
	default:
		field.Value = strings.TrimSpace(m.form.tmp[0])
	}
	if field.Value == "" {
		return
	}
	if row.add {
		m.form.card.Add(row.key, field)
	} else {
		m.form.card[row.key][row.index] = field
	}
}

func (m *model) deleteEditorRow(rows []editorRow) {
	if len(rows) == 0 || m.form.cursor >= len(rows) {
		return
	}
	row := rows[m.form.cursor]
	if row.section || row.add || row.key == "addressbook" || row.key == vcard.FieldFormattedName ||
		row.key == vcard.FieldKind || row.key == vcard.FieldNote || row.key == vcard.FieldBirthday ||
		row.key == vcard.FieldAnniversary || isNameKey(row.key) {
		return
	}
	m.form.card[row.key] = removeField(m.form.card[row.key], row.index)
	if len(m.form.card[row.key]) == 0 {
		delete(m.form.card, row.key)
	}
	rows = m.editorRows()
	m.form.cursor = min(m.form.cursor, len(rows)-1)
}

func popup(fields ...huh.Field) *huh.Form {
	return huh.NewForm(huh.NewGroup(fields...)).WithShowHelp(true).WithShowErrors(true)
}
func required(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New(label + " is required")
		}
		return nil
	}
}
func optionalDate(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return errors.New("date must use YYYY-MM-dd")
	}
	return nil
}
func validEmail(value string) error {
	value = strings.TrimSpace(value)
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return errors.New("enter a valid email address")
	}
	return nil
}
func privateName(value string) error {
	value = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(value)), "X-")
	if value == "" {
		return errors.New("property name is required")
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
			return errors.New("property name must contain letters, digits, or hyphens")
		}
	}
	return nil
}
func scalarValue(card vcard.Card, key string) string {
	if !isNameKey(key) {
		if key == vcard.FieldBirthday || key == vcard.FieldAnniversary {
			return displayDate(card.Value(key))
		}
		return card.Value(key)
	}
	name := card.Name()
	if name == nil {
		return ""
	}
	switch key {
	case "name-prefix":
		return name.HonorificPrefix
	case "name-first":
		return name.GivenName
	case "name-additional":
		return name.AdditionalName
	case "name-last":
		return name.FamilyName
	case "name-suffix":
		return name.HonorificSuffix
	}
	return ""
}
func displayDate(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse("20060102", value); err == nil {
		return parsed.Format("2006-01-02")
	}
	return value
}
func setNameComponent(card vcard.Card, key, value string) {
	name := card.Name()
	if name == nil {
		name = &vcard.Name{}
	}
	switch key {
	case "name-prefix":
		name.HonorificPrefix = value
	case "name-first":
		name.GivenName = value
	case "name-additional":
		name.AdditionalName = value
	case "name-last":
		name.FamilyName = value
	case "name-suffix":
		name.HonorificSuffix = value
	}
	card.SetName(name)
}
func formattedNameFromCard(card vcard.Card) string {
	name := card.Name()
	if name == nil {
		return ""
	}
	return strings.Join(nonEmpty(name.HonorificPrefix, name.GivenName, name.AdditionalName, name.FamilyName, name.HonorificSuffix), " ")
}
func setScalar(card vcard.Card, key, value string) {
	if value == "" {
		delete(card, key)
	} else {
		card.SetValue(key, value)
	}
}
func displayField(key string, field *vcard.Field) string {
	if key == vcard.FieldAddress {
		return strings.Join(nonEmpty(splitComponents(field.Value, 7)...), ", ")
	}
	if key == vcard.FieldOrganization {
		return strings.Join(nonEmpty(splitComponents(field.Value, 2)...), " / ")
	}
	return field.Value
}
func indexedLabel(label string, field *vcard.Field, index int) string {
	if types := field.Params.Types(); len(types) > 0 {
		return fmt.Sprintf("%s (%s)", label, strings.Join(types, ", "))
	}
	return fmt.Sprintf("%s %d", label, index+1)
}
func privateKeys(card vcard.Card) []string {
	var keys []string
	for key := range card {
		if strings.HasPrefix(strings.ToUpper(key), "X-") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
func setFieldType(field *vcard.Field, typ string) {
	if typ = strings.TrimSpace(typ); typ != "" {
		field.Params = vcard.Params{vcard.ParamType: []string{typ}}
	}
}
func removeField(fields []*vcard.Field, index int) []*vcard.Field {
	if index < 0 || index >= len(fields) {
		return fields
	}
	return append(fields[:index:index], fields[index+1:]...)
}
func splitComponents(value string, count int) []string {
	parts := strings.Split(value, ";")
	for len(parts) < count {
		parts = append(parts, "")
	}
	return parts[:count]
}
func isNameKey(key string) bool { return strings.HasPrefix(key, "name-") }
func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func nonEmpty(values ...string) []string {
	var out []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

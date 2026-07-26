package contact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/config"
)

func TestSaveLoadAndDelete(t *testing.T) {
	dir := t.TempDir()
	book := config.Source{Path: dir, Type: "addressbook", DisplayName: "Personal"}
	store := NewStore(&config.Config{Sources: []config.Source{book}})
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	card.AddValue(vcard.FieldEmail, "ada@example.net")
	path, err := store.Save(card, book, "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(path) != ".vcf" {
		t.Fatalf("unexpected path %q", path)
	}
	got, err := store.Load()
	if err != nil || len(got) != 1 {
		t.Fatalf("Load() = %v, %v", got, err)
	}
	if got[0].Name() != "Ada Lovelace" || got[0].Emails()[0] != "ada@example.net" {
		t.Fatalf("unexpected contact: %#v", got[0])
	}
	if err := store.Delete(got[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("contact still exists: %v", err)
	}
}

func TestMergeListsAndConflicts(t *testing.T) {
	a, b := make(vcard.Card), make(vcard.Card)
	a.SetValue(vcard.FieldFormattedName, "Ada L.")
	a.AddValue(vcard.FieldEmail, "ada@one.example")
	b.SetValue(vcard.FieldFormattedName, "Ada Lovelace")
	b.AddValue(vcard.FieldEmail, "ada@two.example")
	a.AddValue(vcard.FieldOrganization, "Engine Society;Research")
	b.AddValue(vcard.FieldOrganization, "Royal Society;Mathematics")
	a.AddValue("X-JABBER", "ada@chat.example")
	b.AddValue("X-JABBER", "lovelace@chat.example")
	conflicts := Conflicts([]vcard.Card{a, b})
	if len(conflicts[vcard.FieldFormattedName]) != 2 {
		t.Fatalf("missing name conflict: %#v", conflicts)
	}
	merged := Merge([]vcard.Card{a, b}, map[string]string{vcard.FieldFormattedName: "Ada Lovelace"})
	if merged.Value(vcard.FieldFormattedName) != "Ada Lovelace" || len(merged.Values(vcard.FieldEmail)) != 2 ||
		len(merged.Values(vcard.FieldOrganization)) != 2 || len(merged.Values("X-JABBER")) != 2 {
		t.Fatalf("bad merge: %#v", merged)
	}
	if merged.Value(vcard.FieldUID) == "" {
		t.Fatal("merged contact has no UID")
	}
}

func TestMergeResolvesNameComponentsAndSingletons(t *testing.T) {
	a, b := make(vcard.Card), make(vcard.Card)
	a.SetName(&vcard.Name{GivenName: "Alex", FamilyName: "Sanson"})
	b.SetName(&vcard.Name{GivenName: "Alejandro", FamilyName: "Sanson", HonorificSuffix: "Jr."})
	a.SetValue(vcard.FieldBirthday, "20100119")
	b.SetValue(vcard.FieldBirthday, "2010-01-19")
	conflicts := Conflicts([]vcard.Card{a, b})
	if got := conflicts["name-first"]; len(got) != 2 {
		t.Fatalf("first-name conflicts = %#v", got)
	}
	if _, exists := conflicts["name-last"]; exists {
		t.Fatalf("equal last names reported as conflict: %#v", conflicts)
	}
	if _, exists := conflicts[vcard.FieldBirthday]; exists {
		t.Fatalf("equivalent birthdays reported as conflict: %#v", conflicts)
	}
	merged := Merge([]vcard.Card{a, b}, map[string]string{"name-first": "Alejandro"})
	if got := merged.Name(); got.GivenName != "Alejandro" || got.FamilyName != "Sanson" || got.HonorificSuffix != "Jr." {
		t.Fatalf("merged name = %#v", got)
	}
}

func TestPreferredContactValuesUseTypeScoreAndAlphabeticalTieBreak(t *testing.T) {
	card := make(vcard.Card)
	card.Add(vcard.FieldEmail, typedField("z-custom@example.net", "school"))
	card.Add(vcard.FieldEmail, typedField("home@example.net", "home"))
	card.Add(vcard.FieldEmail, typedField("work@example.net", "work"))
	card.Add(vcard.FieldEmail, typedField("preferred@example.net", "pref"))
	card.Add(vcard.FieldTelephone, typedField("+81-work", "work"))
	card.Add(vcard.FieldTelephone, typedField("+81-cell-z", "cell"))
	card.Add(vcard.FieldTelephone, typedField("+81-cell-a", "cell"))
	card.Add(vcard.FieldTelephone, typedField("+81-fax", "fax"))
	entry := Contact{Card: card}
	if got := entry.PreferredEmail(); got != "preferred@example.net" {
		t.Fatalf("PreferredEmail() = %q", got)
	}
	if got := entry.PreferredPhone(); got != "+81-cell-a" {
		t.Fatalf("PreferredPhone() = %q", got)
	}
}

func TestContactFieldScoreAddsMultipleTypes(t *testing.T) {
	field := typedField("number", "pref", "fax", "work")
	if got, want := contactFieldScore(field), 80; got != want {
		t.Fatalf("contactFieldScore() = %d, want %d", got, want)
	}
}

func typedField(value string, types ...string) *vcard.Field {
	return &vcard.Field{Value: value, Params: vcard.Params{vcard.ParamType: types}}
}

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadsKhalAddressbooksAndContactTypes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	book := filepath.Join(dir, "contacts")
	data := `[calendars]
[[Work]]
path = ` + filepath.Join(dir, "calendar") + `
type = calendar

[[Friends]]
path = ` + book + `
type = birthdays
readonly = true

[go-khard]
phone_types = satellite, HOME
email_types = school
address_types = vacation
`
	writeTestFile(t, path, data)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	books := cfg.Addressbooks()
	if len(books) != 1 || books[0].Name() != "Friends" || books[0].Path != book {
		t.Fatalf("Addressbooks() = %#v", books)
	}
	if got := cfg.PhoneTypes(); got[len(got)-1] != "satellite" {
		t.Fatalf("PhoneTypes() = %#v", got)
	}
	if got := cfg.EmailTypes(); got[len(got)-1] != "school" {
		t.Fatalf("EmailTypes() = %#v", got)
	}
	if got := cfg.AddressTypes(); got[len(got)-1] != "vacation" {
		t.Fatalf("AddressTypes() = %#v", got)
	}
}

func TestLoadDefaultPrefersKhalAndFallsBackToJSON(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	legacyBook := filepath.Join(home, "legacy")
	writeTestFile(t, LegacyJSONPath(), `{"sources":[{"path":"`+legacyBook+`","type":"addressbook","display_name":"Legacy"}]}`)
	cfg, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if books := cfg.Addressbooks(); len(books) != 1 || books[0].Name() != "Legacy" {
		t.Fatalf("legacy Addressbooks() = %#v", books)
	}

	khalBook := filepath.Join(home, "khal")
	writeTestFile(t, DefaultPath(), "[calendars]\n[[Khal]]\npath = "+khalBook+"\ntype = birthdays\n")
	cfg, err = LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if books := cfg.Addressbooks(); len(books) != 1 || books[0].Name() != "Khal" {
		t.Fatalf("khal Addressbooks() = %#v", books)
	}

	writeTestFile(t, DefaultPath(), "[calendars]\n[[Broken]]\ntype = birthdays\n")
	if _, err := LoadDefault(); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("LoadDefault() error = %v", err)
	}
}

func TestLoadDefaultReportsMissingPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	_, err := LoadDefault()
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), DefaultPath()) || !strings.Contains(err.Error(), LegacyJSONPath()) {
		t.Fatalf("LoadDefault() error = %v", err)
	}
}

func TestLoadExplicitPathDoesNotUseFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	writeTestFile(t, LegacyJSONPath(), `{"sources":[]}`)

	if _, err := Load(DefaultPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load(DefaultPath()) error = %v", err)
	}
}

func TestConfiguredContactTypesExtendDefaults(t *testing.T) {
	cfg := Default()
	cfg.ContactTypes.Phone = []string{"satellite", "HOME"}
	cfg.ContactTypes.Email = []string{"school"}
	if got := cfg.PhoneTypes(); got[len(got)-1] != "satellite" {
		t.Fatalf("PhoneTypes() = %#v", got)
	}
	if got := cfg.EmailTypes(); got[len(got)-1] != "school" {
		t.Fatalf("EmailTypes() = %#v", got)
	}
}

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

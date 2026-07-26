package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadsGoKhalSchema(t *testing.T) {
	dir := t.TempDir()
	book := filepath.Join(dir, "contacts")
	path := filepath.Join(dir, "config.json")
	data := `{"sources":[{"path":"` + book + `","type":"addressbook","display_name":"Friends"},{"path":"` + filepath.Join(dir, "calendar") + `","type":"calendar"}],"default_view":"agenda"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Addressbooks(); len(got) != 1 || got[0].Name() != "Friends" {
		t.Fatalf("Addressbooks() = %#v", got)
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

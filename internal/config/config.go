package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source and Config deliberately match go-khal's configuration schema.
type Source struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name,omitempty"`
	Color       string `json:"color,omitempty"`
	Email       string `json:"email,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
}

type Config struct {
	Sources                   []Source     `json:"sources"`
	DefaultView               string       `json:"default_view,omitempty"`
	WeekStartsOn              string       `json:"week_starts_on,omitempty"`
	TimeFormat                string       `json:"time_format,omitempty"`
	SidebarWidth              int          `json:"sidebar_width,omitempty"`
	RecurrenceLookbackMonths  int          `json:"recurrence_lookback_months,omitempty"`
	RecurrenceLookaheadMonths int          `json:"recurrence_lookahead_months,omitempty"`
	ContactTypes              ContactTypes `json:"contact_types,omitempty"`
}

type ContactTypes struct {
	Phone   []string `json:"phone,omitempty"`
	Email   []string `json:"email,omitempty"`
	Address []string `json:"address,omitempty"`
}

func DefaultPath() string {
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join("khal", "config")
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "khal", "config")
}

func LegacyJSONPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".go-khal.json"
	}
	return filepath.Join(home, ".config", "go-khal", "config.json")
}

func Default() *Config {
	return &Config{DefaultView: "agenda", WeekStartsOn: "monday", TimeFormat: "15:04", SidebarWidth: 30, RecurrenceLookbackMonths: 12, RecurrenceLookaheadMonths: 24}
}

func LoadDefault() (*Config, error) {
	primary := DefaultPath()
	cfg, err := Load(primary)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	legacy := LegacyJSONPath()
	cfg, err = Load(legacy)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no configuration found at %s or deprecated %s: %w", primary, legacy, os.ErrNotExist)
	}
	return nil, fmt.Errorf("load deprecated config %s: %w", legacy, err)
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg *Config
	if strings.EqualFold(filepath.Ext(path), ".json") || bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		cfg, err = loadJSON(data)
	} else {
		cfg, err = loadKhalConfig(data)
	}
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func loadJSON(data []byte) (*Config, error) {
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if err := normalize(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func normalize(cfg *Config) error {
	for i := range cfg.Sources {
		source := &cfg.Sources[i]
		source.Path = filepath.Clean(strings.TrimSpace(source.Path))
		source.Type = strings.ToLower(strings.TrimSpace(source.Type))
		if !filepath.IsAbs(source.Path) {
			return fmt.Errorf("source %d path must be absolute: %s", i+1, source.Path)
		}
		if source.Type != "calendar" && source.Type != "addressbook" {
			return fmt.Errorf("source %d type must be calendar or addressbook", i+1)
		}
	}
	return nil
}

func (s Source) Name() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return filepath.Base(s.Path)
}

func (c *Config) Addressbooks() []Source {
	var out []Source
	for _, s := range c.Sources {
		if s.Type == "addressbook" && !s.Hidden {
			out = append(out, s)
		}
	}
	return out
}

func (c *Config) PhoneTypes() []string {
	return typeList([]string{"cell", "home", "work", "voice", "fax", "pager", "pref"}, c.ContactTypes.Phone)
}
func (c *Config) EmailTypes() []string {
	return typeList([]string{"home", "work", "internet", "pref"}, c.ContactTypes.Email)
}
func (c *Config) AddressTypes() []string {
	return typeList([]string{"home", "work", "pref"}, c.ContactTypes.Address)
}
func typeList(standard, configured []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, values := range [][]string{standard, configured} {
		for _, value := range values {
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" && !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	return out
}

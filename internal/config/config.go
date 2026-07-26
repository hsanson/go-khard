package config

import (
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
	h, err := os.UserHomeDir()
	if err != nil {
		return ".go-khal.json"
	}
	return filepath.Join(h, ".config", "go-khal", "config.json")
}

func Default() *Config {
	return &Config{DefaultView: "agenda", WeekStartsOn: "monday", TimeFormat: "15:04", SidebarWidth: 30, RecurrenceLookbackMonths: 12, RecurrenceLookaheadMonths: 24}
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := Default()
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for i := range cfg.Sources {
		s := &cfg.Sources[i]
		s.Path = filepath.Clean(strings.TrimSpace(s.Path))
		s.Type = strings.ToLower(strings.TrimSpace(s.Type))
		if !filepath.IsAbs(s.Path) {
			return nil, fmt.Errorf("source %d path must be absolute: %s", i+1, s.Path)
		}
		if s.Type != "calendar" && s.Type != "addressbook" {
			return nil, fmt.Errorf("source %d type must be calendar or addressbook", i+1)
		}
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
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

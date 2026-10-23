package config

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

type khalSection struct {
	level  int
	name   string
	parent string
	start  int
	end    int
}

func loadKhalConfig(data []byte) (*Config, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	sections := khalSections(lines)
	cfg := Default()
	foundCalendars := false

	for _, section := range sections {
		if section.level != 1 {
			continue
		}
		switch {
		case strings.EqualFold(section.name, "calendars"):
			foundCalendars = true
		case strings.EqualFold(section.name, "go-khard"):
			values := khalValues(lines, section)
			cfg.ContactTypes.Phone = parseINIList(values["phone_types"])
			cfg.ContactTypes.Email = parseINIList(values["email_types"])
			cfg.ContactTypes.Address = parseINIList(values["address_types"])
		}
	}
	if !foundCalendars {
		return nil, errors.New("missing [calendars] section")
	}

	calendarCount := 0
	for _, section := range sections {
		if section.level != 2 || !strings.EqualFold(section.parent, "calendars") {
			continue
		}
		calendarCount++
		values := khalValues(lines, section)
		rawPath := strings.TrimSpace(values["path"])
		if rawPath == "" {
			return nil, fmt.Errorf("calendar %q: path is required", section.name)
		}
		path, err := expandConfigPath(rawPath)
		if err != nil {
			return nil, fmt.Errorf("calendar %q: %w", section.name, err)
		}
		if _, err := parseINIBool(values["readonly"]); err != nil {
			return nil, fmt.Errorf("calendar %q: readonly: %w", section.name, err)
		}

		typeName := strings.ToLower(strings.TrimSpace(values["type"]))
		if typeName == "" {
			typeName = "calendar"
		}
		switch typeName {
		case "calendar":
			if hasGlobMeta(path) {
				return nil, fmt.Errorf("calendar %q: path globs require type = discover", section.name)
			}
		case "discover":
			// Calendar discovery belongs to go-khal; go-khard consumes addressbooks only.
		case "birthdays":
			if hasGlobMeta(path) {
				return nil, fmt.Errorf("calendar %q: birthday paths cannot contain globs", section.name)
			}
			cfg.Sources = append(cfg.Sources, Source{Path: path, Type: "addressbook", DisplayName: section.name})
		default:
			return nil, fmt.Errorf("calendar %q: unsupported type %q", section.name, typeName)
		}
	}
	if calendarCount == 0 {
		return nil, errors.New("[calendars] must contain at least one [[calendar]] subsection")
	}
	return cfg, nil
}

func expandConfigPath(value string) (string, error) {
	value = os.ExpandEnv(strings.TrimSpace(value))
	if value == "" {
		return "", errors.New("path is empty after expansion")
	}
	if value[0] == '~' {
		slash := strings.IndexAny(value, `/\\`)
		account := value[1:]
		rest := ""
		if slash >= 0 {
			account = value[1:slash]
			rest = value[slash+1:]
		}
		var home string
		if account == "" {
			current, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("resolve home directory: %w", err)
			}
			home = current
		} else {
			entry, err := user.Lookup(account)
			if err != nil {
				return "", fmt.Errorf("resolve user %q: %w", account, err)
			}
			home = entry.HomeDir
		}
		value = filepath.Join(home, rest)
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", value, err)
	}
	return filepath.Clean(absolute), nil
}

func khalSections(lines []string) []khalSection {
	var sections []khalSection
	var stack []int
	tripleQuote := ""
	for lineNumber, line := range lines {
		if tripleQuote != "" {
			if strings.Count(line, tripleQuote)%2 == 1 {
				tripleQuote = ""
			}
			continue
		}
		if _, raw, ok := parseAssignment(line); ok {
			trimmed := strings.TrimSpace(raw)
			if strings.HasPrefix(trimmed, `"""`) && strings.Count(trimmed, `"""`)%2 == 1 {
				tripleQuote = `"""`
			} else if strings.HasPrefix(trimmed, "'''") && strings.Count(trimmed, "'''")%2 == 1 {
				tripleQuote = "'''"
			}
			continue
		}
		level, name, ok := parseSectionHeader(line)
		if !ok {
			continue
		}
		for len(stack) > 0 && sections[stack[len(stack)-1]].level >= level {
			sections[stack[len(stack)-1]].end = lineNumber
			stack = stack[:len(stack)-1]
		}
		parent := ""
		if len(stack) > 0 {
			parent = sections[stack[len(stack)-1]].name
		}
		sections = append(sections, khalSection{level: level, name: name, parent: parent, start: lineNumber, end: len(lines)})
		stack = append(stack, len(sections)-1)
	}
	for _, index := range stack {
		sections[index].end = len(lines)
	}
	return sections
}

func khalValues(lines []string, section khalSection) map[string]string {
	values := make(map[string]string)
	for lineNumber := section.start + 1; lineNumber < section.end; lineNumber++ {
		if _, _, ok := parseSectionHeader(lines[lineNumber]); ok {
			break
		}
		key, raw, ok := parseAssignment(lines[lineNumber])
		if ok {
			values[strings.ToLower(key)] = parseINIValue(raw)
		}
	}
	return values
}

func parseSectionHeader(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(stripInlineComment(line))
	if !strings.HasPrefix(trimmed, "[") {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '[' {
		level++
	}
	closing := 0
	for closing < len(trimmed) && trimmed[len(trimmed)-1-closing] == ']' {
		closing++
	}
	if closing != level || len(trimmed) <= level+closing {
		return 0, "", false
	}
	name := unquoteINI(strings.TrimSpace(trimmed[level : len(trimmed)-closing]))
	return level, name, name != ""
}

func parseAssignment(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	equals := unquotedIndex(line, '=')
	if equals < 0 {
		return "", "", false
	}
	key := unquoteINI(strings.TrimSpace(line[:equals]))
	return key, line[equals+1:], key != ""
}

func parseINIValue(raw string) string {
	return unquoteINI(strings.TrimSpace(stripInlineComment(raw)))
}

func stripInlineComment(value string) string {
	if index := unquotedIndex(value, '#'); index >= 0 {
		return value[:index]
	}
	return value
}

func unquotedIndex(value string, target byte) int {
	quote := byte(0)
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\'', '"':
			if quote == 0 {
				quote = value[index]
			} else if quote == value[index] && (index == 0 || value[index-1] != '\\') {
				quote = 0
			}
		default:
			if value[index] == target && quote == 0 {
				return index
			}
		}
	}
	return -1
}

func unquoteINI(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != value[len(value)-1] || (value[0] != '\'' && value[0] != '"') {
		return value
	}
	if value[0] == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	return strings.ReplaceAll(value[1:len(value)-1], `\'`, `'`)
}

func parseINIList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var values []string
	start := 0
	quote := byte(0)
	for index := 0; index <= len(value); index++ {
		if index == len(value) || (value[index] == ',' && quote == 0) {
			item := unquoteINI(strings.TrimSpace(value[start:index]))
			if item != "" {
				values = append(values, item)
			}
			start = index + 1
			continue
		}
		if value[index] == '\'' || value[index] == '"' {
			if quote == 0 {
				quote = value[index]
			} else if quote == value[index] && (index == 0 || value[index-1] != '\\') {
				quote = 0
			}
		}
	}
	return values
}

func parseINIBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "false", "off", "no", "0":
		return false, nil
	case "true", "on", "yes", "1":
		return true, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}

func hasGlobMeta(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

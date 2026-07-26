package contact

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khard/internal/config"
)

type Contact struct {
	Card vcard.Card
	Path string
	Book config.Source
}

func (c Contact) Name() string {
	if v := strings.TrimSpace(c.Card.Value(vcard.FieldFormattedName)); v != "" {
		return v
	}
	return strings.TrimSpace(c.Card.Value(vcard.FieldOrganization))
}
func (c Contact) Emails() []string { return clean(c.Card.Values(vcard.FieldEmail)) }
func (c Contact) Phones() []string { return clean(c.Card.Values(vcard.FieldTelephone)) }
func (c Contact) SearchText() string {
	return strings.ToLower(strings.Join(append(append([]string{c.Name()}, c.Emails()...), c.Phones()...), " "))
}

type Store struct{ Config *config.Config }

func NewStore(cfg *config.Config) *Store { return &Store{Config: cfg} }

func (s *Store) Load() ([]Contact, error) {
	var out []Contact
	for _, book := range s.Config.Addressbooks() {
		entries, err := os.ReadDir(book.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", book.Path, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".vcf") {
				continue
			}
			path := filepath.Join(book.Path, e.Name())
			f, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			dec := vcard.NewDecoder(f)
			for {
				card, decodeErr := dec.Decode()
				if decodeErr == io.EOF {
					break
				}
				if decodeErr != nil {
					_ = f.Close()
					return nil, fmt.Errorf("decode %s: %w", path, decodeErr)
				}
				out = append(out, Contact{Card: card, Path: path, Book: book})
			}
			_ = f.Close()
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name()) < strings.ToLower(out[j].Name()) })
	return out, nil
}

func (s *Store) Save(card vcard.Card, book config.Source, existing string) (string, error) {
	if err := os.MkdirAll(book.Path, 0o755); err != nil {
		return "", err
	}
	if card.Value(vcard.FieldVersion) == "" {
		card.SetValue(vcard.FieldVersion, "4.0")
	}
	if card.Value(vcard.FieldUID) == "" {
		card.SetValue(vcard.FieldUID, newID())
	}
	card.SetValue(vcard.FieldRevision, time.Now().UTC().Format("20060102T150405Z"))
	path := existing
	if path == "" || filepath.Clean(filepath.Dir(path)) != filepath.Clean(book.Path) {
		path = filepath.Join(book.Path, safeName(card.Value(vcard.FieldUID))+".vcf")
	}
	tmp, err := os.CreateTemp(book.Path, ".go-khard-*.vcf")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err = vcard.NewEncoder(tmp).Encode(card); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) Delete(c Contact) error { return os.Remove(c.Path) }

func Clone(card vcard.Card) vcard.Card {
	out := make(vcard.Card, len(card))
	for k, fields := range card {
		for _, f := range fields {
			cp := *f
			cp.Params = cloneParams(f.Params)
			out[k] = append(out[k], &cp)
		}
	}
	return out
}

func Merge(cards []vcard.Card, scalarChoices map[string]string) vcard.Card {
	out := make(vcard.Card)
	for _, card := range cards {
		for key, fields := range card {
			if key == vcard.FieldUID || key == vcard.FieldRevision || key == vcard.FieldVersion {
				continue
			}
			if key == vcard.FieldName {
				continue
			}
			if isListField(key) {
				seen := map[string]bool{}
				for _, old := range out[key] {
					seen[old.Value] = true
				}
				for _, f := range fields {
					if !seen[f.Value] {
						cp := *f
						cp.Params = cloneParams(f.Params)
						out.Add(key, &cp)
						seen[f.Value] = true
					}
				}
			} else if len(out[key]) == 0 {
				for _, f := range fields {
					cp := *f
					cp.Params = cloneParams(f.Params)
					out.Add(key, &cp)
				}
			}
		}
	}
	name := mergeName(cards)
	for key, value := range scalarChoices {
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
		default:
			if value != "" {
				out.SetValue(key, value)
			}
		}
	}
	if name.FamilyName != "" || name.GivenName != "" || name.AdditionalName != "" ||
		name.HonorificPrefix != "" || name.HonorificSuffix != "" {
		out.SetName(name)
	}
	for _, key := range []string{vcard.FieldBirthday, vcard.FieldAnniversary} {
		if value := out.Value(key); value != "" {
			out.SetValue(key, normalizedScalarValue(key, value))
		}
	}
	out.SetValue(vcard.FieldUID, newID())
	out.SetValue(vcard.FieldVersion, "4.0")
	return out
}

func Conflicts(cards []vcard.Card) map[string][]string {
	all := map[string][]string{}
	for _, c := range cards {
		for k, fs := range c {
			if k == vcard.FieldName {
				continue
			}
			if isListField(k) || k == vcard.FieldUID || k == vcard.FieldRevision || k == vcard.FieldVersion {
				continue
			}
			for _, f := range fs {
				value := normalizedScalarValue(k, f.Value)
				if value != "" && !contains(all[k], value) {
					all[k] = append(all[k], value)
				}
			}
		}
	}
	for _, key := range []string{"name-prefix", "name-first", "name-additional", "name-last", "name-suffix"} {
		for _, card := range cards {
			value := nameComponent(card.Name(), key)
			if value != "" && !contains(all[key], value) {
				all[key] = append(all[key], value)
			}
		}
	}
	for k, values := range all {
		if len(values) < 2 {
			delete(all, k)
		}
	}
	return all
}
func normalizedScalarValue(key, value string) string {
	value = strings.TrimSpace(value)
	if key == vcard.FieldBirthday || key == vcard.FieldAnniversary {
		if parsed, err := time.Parse("20060102", value); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return value
}
func mergeName(cards []vcard.Card) *vcard.Name {
	name := &vcard.Name{}
	for _, card := range cards {
		current := card.Name()
		if current == nil {
			continue
		}
		if name.HonorificPrefix == "" {
			name.HonorificPrefix = current.HonorificPrefix
		}
		if name.GivenName == "" {
			name.GivenName = current.GivenName
		}
		if name.AdditionalName == "" {
			name.AdditionalName = current.AdditionalName
		}
		if name.FamilyName == "" {
			name.FamilyName = current.FamilyName
		}
		if name.HonorificSuffix == "" {
			name.HonorificSuffix = current.HonorificSuffix
		}
	}
	return name
}
func nameComponent(name *vcard.Name, key string) string {
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
	default:
		return ""
	}
}
func isListField(key string) bool {
	switch key {
	case vcard.FieldEmail, vcard.FieldTelephone, vcard.FieldAddress, vcard.FieldURL,
		vcard.FieldNickname, vcard.FieldCategories, vcard.FieldOrganization,
		vcard.FieldTitle, vcard.FieldRole:
		return true
	default:
		return strings.HasPrefix(strings.ToUpper(key), "X-")
	}
}

func clean(in []string) []string {
	var out []string
	for _, v := range in {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func cloneParams(in vcard.Params) vcard.Params {
	out := make(vcard.Params, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}
func newID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '-'
		}
		return r
	}, s)
	if s == "" {
		return newID()
	}
	return s
}

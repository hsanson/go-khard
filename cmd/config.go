package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hsanson/go-khard/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Manage shared go-khal/go-khard configuration"}
	c.AddCommand(&cobra.Command{Use: "init", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		if err := config.Save(cfgPath, cfg); err != nil {
			return err
		}
		fmt.Printf("initialized config at %s\n", cfgPath)
		return nil
	}})
	c.AddCommand(&cobra.Command{Use: "list-addressbooks", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		for _, s := range cfg.Addressbooks() {
			fmt.Printf("%s\t%s\n", s.Name(), s.Path)
		}
		return nil
	}})
	c.AddCommand(newFromVdirsyncerCommand())
	return c
}

func newFromVdirsyncerCommand() *cobra.Command {
	return &cobra.Command{Use: "from-vdirsyncer [path]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		path := filepath.Join(userHome(), ".config", "vdirsyncer", "config")
		if len(args) > 0 {
			path = args[0]
		}
		path = expandHome(path)
		sources, err := sourcesFromVdirsyncer(path)
		if err != nil {
			return err
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			cfg = config.Default()
		}
		// Match go-khal: generation replaces all discovered sources.
		cfg.Sources = sources
		if err := config.Save(cfgPath, cfg); err != nil {
			return err
		}
		fmt.Printf("wrote %d sources to %s\n", len(sources), cfgPath)
		return nil
	}}
}

type storage struct{ typ, path, ext string }

func sourcesFromVdirsyncer(path string) ([]config.Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open vdirsyncer config: %w", err)
	}
	defer func() { _ = f.Close() }()
	var stores []storage
	var cur *storage
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if cur != nil {
				stores = append(stores, *cur)
			}
			cur = nil
			section := strings.TrimSpace(strings.Trim(line, "[]"))
			if strings.HasPrefix(section, "storage ") {
				cur = &storage{}
			}
			continue
		}
		if cur == nil {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.Trim(strings.TrimSpace(strings.SplitN(val, "#", 2)[0]), `"'`)
		switch strings.TrimSpace(key) {
		case "type":
			cur.typ = val
		case "path":
			cur.path = val
		case "fileext":
			cur.ext = val
		}
	}
	if cur != nil {
		stores = append(stores, *cur)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []config.Source
	for _, st := range stores {
		if strings.ToLower(st.typ) != "filesystem" {
			continue
		}
		root, err := filepath.Abs(expandHome(st.path))
		if err != nil {
			continue
		}
		typ := ""
		switch strings.ToLower(st.ext) {
		case ".vcf":
			typ = "addressbook"
		case ".ics":
			typ = "calendar"
		}
		if typ == "" {
			typ = detectType(root)
		}
		if typ == "" {
			continue
		}
		for _, dir := range concreteDirs(root, typ) {
			if seen[dir] {
				continue
			}
			seen[dir] = true
			name, color := metadata(dir)
			out = append(out, config.Source{Path: dir, Type: typ, DisplayName: name, Color: color})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].Path < out[j].Path
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}
func concreteDirs(root, typ string) []string {
	ext := ".ics"
	if typ == "addressbook" {
		ext = ".vcf"
	}
	if hasExt(root, ext) || hasMeta(root) {
		return []string{root}
	}
	es, _ := os.ReadDir(root)
	var out []string
	for _, e := range es {
		if e.IsDir() {
			p := filepath.Join(root, e.Name())
			if hasExt(p, ext) || hasMeta(p) {
				out = append(out, p)
			}
		}
	}
	return out
}
func detectType(p string) string {
	if hasExt(p, ".ics") {
		return "calendar"
	}
	if hasExt(p, ".vcf") {
		return "addressbook"
	}
	return ""
}
func hasExt(p, e string) bool {
	es, _ := os.ReadDir(p)
	for _, x := range es {
		if !x.IsDir() && strings.EqualFold(filepath.Ext(x.Name()), e) {
			return true
		}
	}
	return false
}
func hasMeta(p string) bool {
	for _, n := range []string{"displayname", ".displayname", "color", ".color"} {
		if _, e := os.Stat(filepath.Join(p, n)); e == nil {
			return true
		}
	}
	return false
}
func metadata(p string) (string, string) {
	return firstFile(p, "displayname", ".displayname"), firstFile(p, "color", ".color")
}
func firstFile(p string, names ...string) string {
	for _, n := range names {
		if b, e := os.ReadFile(filepath.Join(p, n)); e == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}
func userHome() string { h, _ := os.UserHomeDir(); return h }
func expandHome(p string) string {
	if p == "~" {
		return userHome()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(userHome(), p[2:])
	}
	return p
}

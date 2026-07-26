package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/hsanson/go-khard/internal/config"
	"github.com/hsanson/go-khard/internal/contact"
	"github.com/hsanson/go-khard/internal/tui"
	"github.com/spf13/cobra"
)

var cfgPath string

var rootCmd = &cobra.Command{
	Use:   "go-khard [query]",
	Short: "Interactive vCard address book",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		store := contact.NewStore(cfg)
		if len(args) == 1 {
			return runQuery(store, args[0])
		}
		return tui.Run(store, cfg)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath(), "shared go-khal/go-khard config file")
	rootCmd.AddCommand(newConfigCommand())
	rootCmd.AddCommand(newQueryCommand())
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newQueryCommand() *cobra.Command {
	return &cobra.Command{
		Use: "query <text>", Short: "neomutt query_command compatible contact query",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			return runQuery(contact.NewStore(cfg), args[0])
		},
	}
}

func runQuery(store *contact.Store, query string) error {
	contacts, err := store.Load()
	if err != nil {
		return err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	fmt.Println("go-khard query results")
	for _, c := range contacts {
		if q != "" && !fuzzy(c.SearchText(), q) {
			continue
		}
		for _, email := range c.Emails() {
			fmt.Printf("%s\t%s\n", email, c.Name())
		}
	}
	return nil
}

func fuzzy(text, query string) bool {
	i := 0
	q := []rune(query)
	for _, r := range text {
		if i < len(q) && r == q[i] {
			i++
		}
	}
	return i == len(q)
}

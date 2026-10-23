package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Inspect the shared khal configuration"}
	command.AddCommand(&cobra.Command{
		Use:  "list-addressbooks",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			for _, source := range cfg.Addressbooks() {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", source.Name(), source.Path)
			}
			return nil
		},
	})
	return command
}

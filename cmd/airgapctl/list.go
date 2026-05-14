package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List images contained in the bundle",
		Long:  `Extract the bundle and list all container images discovered in airgap.yaml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Bundle: %s\n", globalOpts.bundle)
			fmt.Fprintln(cmd.OutOrStdout(), "Images found in bundle:")
			fmt.Fprintln(cmd.OutOrStdout(), "  (list implementation pending)")
			return nil
		},
	}
	return cmd
}

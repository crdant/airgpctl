package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var globalOpts struct {
	bundle   string
	verbose  bool
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "airgapctl [bundle]",
		Short: "Load Replicated .airgap bundles into a private registry",
		Long: `airgapctl extracts Replicated .airgap bundles, pushes images to a private registry,
and generates Helm values files mapping images to their new paths.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// If --bundle flag is set, it wins. Otherwise use positional arg if provided.
			if globalOpts.bundle == "" && len(args) > 0 {
				globalOpts.bundle = args[0]
			}
			if globalOpts.bundle == "" {
				return fmt.Errorf("bundle path is required (use --bundle or positional argument)")
			}
			// Resolve to absolute path and verify existence.
			abs, err := filepath.Abs(globalOpts.bundle)
			if err != nil {
				return fmt.Errorf("invalid bundle path %q: %w", globalOpts.bundle, err)
			}
		fi, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("bundle file not found: %w", err)
		}
		if fi.IsDir() {
			return fmt.Errorf("bundle path is a directory, expected a file: %s", abs)
		}
			globalOpts.bundle = abs
			return nil
		},
	}

	root.PersistentFlags().StringVar(&globalOpts.bundle, "bundle", "", "path to .airgap bundle file")
	root.PersistentFlags().BoolVar(&globalOpts.verbose, "verbose", false, "enable verbose output")

	root.AddCommand(newListCmd())
	root.AddCommand(newPushCmd())
	root.AddCommand(newValuesCmd())

	return root
}



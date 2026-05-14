package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/replicatedhq/airgapctl/pkg/bundle"
	"github.com/replicatedhq/airgapctl/pkg/distribution"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List images contained in the bundle",
		Long:  `Extract the bundle and list all container images discovered in airgap.yaml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := bundle.OpenBundle(globalOpts.bundle)
			if err != nil {
				return fmt.Errorf("opening bundle: %w", err)
			}

			extractDir, err := os.MkdirTemp("", "airgapctl-list-*")
			if err != nil {
				return fmt.Errorf("creating temp directory: %w", err)
			}
			defer os.RemoveAll(extractDir)

			if err := b.ExtractTo(extractDir); err != nil {
				return fmt.Errorf("extracting bundle: %w", err)
			}

			walker := distribution.NewWalker(extractDir)
			images, err := walker.ResolveImages(b.Spec.SavedImages)
			if err != nil {
				return fmt.Errorf("resolving images: %w", err)
			}

			if len(images) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No images found in bundle.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "IMAGE\tTYPE\tDIGEST\tPLATFORMS")
			for _, img := range images {
				imgType := "single-arch"
				platforms := "-"
				if img.IsMultiArch {
					imgType = "multi-arch"
					var parts []string
					for _, m := range img.Manifests {
						parts = append(parts, m.Platform)
					}
					platforms = fmt.Sprintf("%d (%s)", len(parts), strings.Join(parts, ", "))
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", img.SourceRef, imgType, img.Digest, platforms)
			}
			w.Flush()

			return nil
		},
	}
	return cmd
}

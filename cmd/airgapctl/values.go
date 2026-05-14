package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/replicatedhq/airgapctl/pkg/bundle"
	"github.com/replicatedhq/airgapctl/pkg/distribution"
	"github.com/replicatedhq/airgapctl/pkg/values"
	"github.com/spf13/cobra"
)

var valuesOpts struct {
	chart     string
	registry  string
	namespace string
	output    string
	template  string
}

func newValuesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values",
		Short: "Generate Helm values file with remapped image references",
		Long:  `Read a Helm chart's values.yaml, detect image references, and generate an output YAML with images remapped to the destination registry.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if valuesOpts.chart == "" {
				return fmt.Errorf("--chart is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := bundle.OpenBundle(globalOpts.bundle)
			if err != nil {
				return fmt.Errorf("opening bundle: %w", err)
			}

			extractDir, err := os.MkdirTemp("", "airgapctl-values-*")
			if err != nil {
				return fmt.Errorf("creating temp directory: %w", err)
			}
			defer os.RemoveAll(extractDir)

			if err := b.ExtractTo(extractDir); err != nil {
				return fmt.Errorf("extracting bundle: %w", err)
			}

			walker := distribution.NewWalker(extractDir)
			images, err := walker.ResolveImages(b.SavedImages)
			if err != nil {
				return fmt.Errorf("resolving images: %w", err)
			}

			// Read chart values.yaml to detect image references for filtering
			chartValuesPath := filepath.Join(valuesOpts.chart, "values.yaml")
			chartData, err := os.ReadFile(chartValuesPath)
			if err != nil {
				return fmt.Errorf("reading chart values.yaml: %w", err)
			}

			// Extract potential image references from chart values for filtering
			chartRefs := extractImageRefs(string(chartData))
			matcher := values.NewChartMatcher(chartRefs)

			gen := values.Generator{
				Registry:  valuesOpts.registry,
				Namespace: valuesOpts.namespace,
				Template:  valuesOpts.template,
				Filter:    matcher.Match,
			}

			if err := gen.Generate(images, valuesOpts.output); err != nil {
				return fmt.Errorf("generating values file: %w", err)
			}

			matched := 0
			for _, img := range images {
				if matcher.Match(img.SourceRef) {
					matched++
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Wrote values file to %s (%d/%d images matched)\n", valuesOpts.output, matched, len(images))
			return nil
		},
	}

	cmd.Flags().StringVar(&valuesOpts.chart, "chart", "", "path to Helm chart directory (required)")
	cmd.Flags().StringVar(&valuesOpts.registry, "registry", "", "destination registry URL")
	cmd.Flags().StringVar(&valuesOpts.namespace, "namespace", "", "destination namespace/prefix")
	cmd.Flags().StringVar(&valuesOpts.output, "output", "values-airgap.yaml", "output values file path")
	cmd.Flags().StringVar(&valuesOpts.template, "template", "", "Go template for destination image path")

	_ = cmd.MarkFlagRequired("chart")

	return cmd
}

// extractImageRefs performs a naive extraction of potential image references from chart values text.
func extractImageRefs(data string) []string {
	var refs []string
	// Very simple heuristic: look for lines containing "repository:" or image-like strings
	lines := []string{}
	for _, line := range splitLines(data) {
		lines = append(lines, line)
	}

	for _, line := range lines {
		line = trimSpace(line)
		// Look for repository: values or inline image references
		if idx := findSubstr(line, "repository:"); idx >= 0 {
			repo := trimSpace(line[idx+len("repository:"):])
			if repo != "" {
				refs = append(refs, repo)
			}
		}
		// Also look for image: lines that might contain full references
		if idx := findSubstr(line, "image:"); idx >= 0 {
			img := trimSpace(line[idx+len("image:"):])
			if img != "" && img[0] == '"' {
				img = trimQuotes(img)
				if img != "" {
					refs = append(refs, img)
				}
			}
		}
	}
	return refs
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func findSubstr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

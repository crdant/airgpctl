package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/replicatedhq/airgapctl/pkg/bundle"
	"github.com/replicatedhq/airgapctl/pkg/distribution"
	"github.com/replicatedhq/airgapctl/pkg/values"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var valuesOpts struct {
	chart     string
	version   string
	registry  string
	namespace string
	output    string
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
			images, err := walker.ResolveImages(b.Spec.SavedImages)
			if err != nil {
				return fmt.Errorf("resolving images: %w", err)
			}

		chartDir := valuesOpts.chart
		// If chart is an OCI reference, pull it with helm to a temp directory
		if strings.HasPrefix(chartDir, "oci://") {
			tmpChartDir, err := os.MkdirTemp("", "airgapctl-chart-*")
			if err != nil {
				return fmt.Errorf("creating temp chart directory: %w", err)
			}
			defer os.RemoveAll(tmpChartDir)

			helmArgs := []string{"pull", chartDir, "--untar", "--untardir", tmpChartDir}
			if valuesOpts.version != "" {
				helmArgs = append(helmArgs, "--version", valuesOpts.version)
			}
			if globalOpts.verbose {
				fmt.Fprintf(cmd.OutOrStdout(), "Pulling chart %s...\n", chartDir)
			}
			out, err := exec.Command("helm", helmArgs...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("helm pull failed: %w\n%s", err, string(out))
			}

			// helm --untar extracts into a subdirectory named after the chart.
			// Find the directory whose Chart.yaml metadata.name matches the chart
			// reference name (last path component of the OCI URL).
			expectedName := filepath.Base(valuesOpts.chart)
			chartDir, err = findChartDir(tmpChartDir, expectedName)
			if err != nil {
				return fmt.Errorf("helm pull did not produce an extracted chart directory: %w", err)
			}
		} else if isChartTarball(chartDir) {
			// Extract local chart tarball to a temp directory
			tmpChartDir, err := os.MkdirTemp("", "airgapctl-chart-*")
			if err != nil {
				return fmt.Errorf("creating temp chart directory: %w", err)
			}
			defer os.RemoveAll(tmpChartDir)

			if err := extractTarball(chartDir, tmpChartDir); err != nil {
				return fmt.Errorf("extracting chart tarball: %w", err)
			}

			// Find the directory whose Chart.yaml metadata.name matches the chart
			// name derived from the tarball filename.
			expectedName := chartNameFromTarball(valuesOpts.chart)
			chartDir, err = findChartDir(tmpChartDir, expectedName)
			if err != nil {
				return fmt.Errorf("chart tarball did not produce an extracted chart directory: %w", err)
			}
		}

		// Read chart values.yaml to detect image references for filtering
		chartValuesPath := filepath.Join(chartDir, "values.yaml")
		chartData, err := os.ReadFile(chartValuesPath)
		if err != nil {
			return fmt.Errorf("reading chart values.yaml: %w", err)
		}

		// Extract potential image references from chart values for filtering
		chartRefs := extractImageRefs(string(chartData))
		matcher := values.NewChartMatcher(chartRefs)

		// Filter images to only those referenced by the chart
		var chartImages []distribution.Image
		for _, img := range images {
			if matcher.Match(imageNameFromRef(img.SourceRef)) {
				chartImages = append(chartImages, img)
			}
		}

		// Parse chart values as nested map for structure-preserving output
		var chartValues map[string]interface{}
		if err := yaml.Unmarshal(chartData, &chartValues); err != nil {
			return fmt.Errorf("parsing chart values.yaml: %w", err)
		}

		remapped := values.RemapChartValues(chartValues, chartImages, valuesOpts.registry, valuesOpts.namespace)

		if err := writeValuesYAML(remapped, valuesOpts.output); err != nil {
			return fmt.Errorf("generating values file: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Wrote values file to %s (%d/%d images matched)\n", valuesOpts.output, len(chartImages), len(images))
		return nil
		},
	}

	cmd.Flags().StringVar(&valuesOpts.chart, "chart", "", "path to Helm chart directory or OCI reference (required)")
	cmd.Flags().StringVar(&valuesOpts.version, "version", "", "chart version (required for OCI charts)")
	cmd.Flags().StringVar(&valuesOpts.registry, "registry", "", "destination registry URL")
	cmd.Flags().StringVar(&valuesOpts.namespace, "namespace", "", "destination namespace/prefix")
	cmd.Flags().StringVar(&valuesOpts.output, "output", "values-airgap.yaml", "output values file path")

	_ = cmd.MarkFlagRequired("chart")

	return cmd
}

// extractImageRefs performs extraction of potential image references from chart values text.
// It looks for repository fields, full image references in arrays, and inline image strings.
func extractImageRefs(data string) []string {
	var refs []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		// Look for repository: values
		if strings.HasPrefix(line, "repository:") {
			repo := strings.TrimSpace(line[len("repository:"):])
			repo = strings.Trim(repo, `"'`)
			if repo != "" {
				refs = append(refs, repo)
			}
		}
		// Look for image: lines that might contain full references
		if strings.HasPrefix(line, "image:") {
			img := strings.TrimSpace(line[len("image:"):])
			if img != "" {
				// Strip surrounding quotes (single or double)
				img = strings.Trim(img, `"'`)
				if img != "" {
					refs = append(refs, img)
				}
			}
		}
		// Look for array items that are full image references (e.g. releaseImages:)
		if strings.HasPrefix(line, "- ") {
			item := strings.TrimPrefix(line, "- ")
			item = strings.TrimSpace(item)
			item = strings.Trim(item, `"'`)
			if item != "" && strings.Contains(item, ":") {
				refs = append(refs, item)
			}
		}
	}
	return refs
}

// isChartTarball returns true if the path looks like a Helm chart tarball.
func isChartTarball(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".tgz" || strings.HasSuffix(path, ".tar.gz")
}

// extractTarball extracts a gzipped tar archive to the destination directory.
func extractTarball(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dst, header.Name)
		// Prevent directory traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar entry: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink, tar.TypeLink:
			// Skip symlinks and hard links to prevent directory traversal attacks
			continue
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			// Skip PAX extended headers
			continue
		}
	}
	return nil
}

// imageNameFromRef extracts the short image name from a full reference or repository path.
func imageNameFromRef(ref string) string {
	// Strip tag if present
	if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
		ref = ref[:idx]
	}
	idx := strings.LastIndex(ref, "/")
	if idx == -1 {
		return ref
	}
	return ref[idx+1:]
}

// findChartDir looks inside root for an immediate subdirectory that contains a
// Chart.yaml whose metadata.name matches expectedName. If none match, it falls
// back to the first subdirectory it finds.
func findChartDir(root string, expectedName string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("reading extracted chart directory: %w", err)
	}

	var firstDir string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(root, entry.Name())
		if firstDir == "" {
			firstDir = dirPath
		}

		chartYAMLPath := filepath.Join(dirPath, "Chart.yaml")
		data, err := os.ReadFile(chartYAMLPath)
		if err != nil {
			continue
		}

		var meta struct {
			Name string `yaml:"name"`
		}
		if err := yaml.Unmarshal(data, &meta); err != nil {
			continue
		}
		if meta.Name == expectedName {
			return dirPath, nil
		}
	}

	if firstDir == "" {
		return "", fmt.Errorf("no chart directory found in %s", root)
	}
	return firstDir, nil
}

// chartNameFromTarball returns the chart name derived from a tarball filename
// by stripping the .tgz or .tar.gz extension.
func chartNameFromTarball(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	if strings.HasSuffix(lower, ".tar.gz") {
		return base[:len(base)-7]
	}
	if strings.HasSuffix(lower, ".tgz") {
		return base[:len(base)-4]
	}
	return base
}

// writeValuesYAML marshals the values map to YAML and writes it to outputPath.
func writeValuesYAML(values map[string]interface{}, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	data, err := marshalYAML(values)
	if err != nil {
		return fmt.Errorf("marshaling values YAML: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("writing values file: %w", err)
	}

	return nil
}

// marshalYAML wraps yaml.Marshal to recover from panics on unmarshalable types.
func marshalYAML(values map[string]interface{}) (data []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			// yaml.Marshal panics on types like channels; convert to error
			data = nil
			err = fmt.Errorf("yaml marshal panic: %v", r)
		}
	}()
	return yaml.Marshal(values)
}

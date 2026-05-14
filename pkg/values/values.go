// Package values provides functionality for generating Helm values files.
package values

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/replicatedhq/airgapctl/pkg/distribution"
	"gopkg.in/yaml.v3"
)

// Generator produces a Helm values file mapping source images to destination registry paths.
type Generator struct {
	Registry    string            // Destination registry host (e.g., "myregistry.example.com")
	Namespace   string            // Destination namespace/prefix (e.g., "myapp")
	TagOverride string            // Optional tag to override all images (empty = preserve original)
	Template    string            // Destination path template; default "{registry}/{namespace}/{name}:{tag}"
	Filter      func(string) bool // Optional filter; images are included only when Filter(name) == true
}

// Generate creates a YAML values file at outputPath mapping the provided images
// to their destination registry references according to the configured template.
func (g *Generator) Generate(images []distribution.Image, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	entries := make(map[string]string, len(images))

	for _, img := range images {
		name := imageName(img.Repository)
		if g.Filter != nil && !g.Filter(name) {
			continue
		}

		tag := img.Tag
		if g.TagOverride != "" {
			tag = g.TagOverride
		}

		dest, err := g.applyTemplate(img.Repository, tag)
		if err != nil {
			return fmt.Errorf("mapping %q: %w", img.SourceRef, err)
		}
		entries[name] = dest
	}

	data, err := yaml.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshaling values YAML: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("writing values file: %w", err)
	}

	return nil
}

// applyTemplate substitutes template variables in g.Template.
func (g *Generator) applyTemplate(repo, tag string) (string, error) {
	tmpl := g.Template
	if tmpl == "" {
		tmpl = "{registry}/{namespace}/{name}:{tag}"
	}

	name := imageName(repo)

	// Build a replacement map
	vars := map[string]string{
		"registry":    g.Registry,
		"namespace":   g.Namespace,
		"name":        name,
		"tag":         tag,
		"repository":  repo,
	}

	result := tmpl
	for k, v := range vars {
		placeholder := "{" + k + "}"
		result = strings.ReplaceAll(result, placeholder, v)
	}

	// Detect any remaining unmatched placeholders like {unknown}
	if strings.Contains(result, "{") && strings.Contains(result, "}") {
		return "", fmt.Errorf("unknown template variable in template %q", tmpl)
	}

	// Clean up double slashes caused by empty namespace
	for strings.Contains(result, "//") {
		result = strings.ReplaceAll(result, "//", "/")
	}

	// Clean up trailing slash before colon in tag separator
	result = strings.ReplaceAll(result, "/:", ":")

	return result, nil
}

// imageName extracts the short image name from a repository path.
// e.g. "library/nginx" → "nginx", "registry.com/ns/app" → "app"
func imageName(repo string) string {
	idx := strings.LastIndex(repo, "/")
	if idx == -1 {
		return repo
	}
	return repo[idx+1:]
}

// ChartMatcher implements partial matching of image names against a list of chart references.
type ChartMatcher struct {
	refs []string
}

// NewChartMatcher creates a matcher that matches an image name if it contains any of the chart refs.
func NewChartMatcher(refs []string) *ChartMatcher {
	return &ChartMatcher{refs: refs}
}

// Match returns true if the given image name matches any chart reference.
func (c *ChartMatcher) Match(name string) bool {
	for _, ref := range c.refs {
		if strings.Contains(name, ref) || strings.Contains(ref, name) {
			return true
		}
	}
	return false
}

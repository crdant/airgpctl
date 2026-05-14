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
	Template    string            // Destination path template; default "{registry}/{namespace}/{path}:{tag}"
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

		dest, err := g.applyTemplate(img.SourceRef, tag)
		if err != nil {
			return fmt.Errorf("mapping %q: %w", img.SourceRef, err)
		}

		if _, exists := entries[name]; exists {
			return fmt.Errorf("duplicate image name %q from repository %q", name, img.Repository)
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
func (g *Generator) applyTemplate(sourceRef, tag string) (string, error) {
	tmpl := g.Template
	if tmpl == "" {
		tmpl = "{registry}/{namespace}/{path}:{tag}"
	}

	name := imageName(sourceRef)
	path := imagePath(sourceRef)
	repo := stripTag(sourceRef)

	// Build a replacement map
	vars := map[string]string{
		"registry":   g.Registry,
		"namespace":  g.Namespace,
		"name":       name,
		"path":       path,
		"tag":        tag,
		"repository": repo,
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

	// Clean up double slashes caused by empty namespace or registry
	for strings.Contains(result, "//") {
		result = strings.ReplaceAll(result, "//", "/")
	}

	// Remove leading slash that appears when registry is empty
	result = strings.TrimPrefix(result, "/")

	// Clean up trailing slash before colon in tag separator
	result = strings.ReplaceAll(result, "/:", ":")

	return result, nil
}

// imageName extracts the short image name from a repository path or full reference.
// e.g. "library/nginx" → "nginx", "registry.com/ns/app:1.0" → "app"
func imageName(repo string) string {
	// Strip tag if present
	if idx := strings.LastIndex(repo, ":"); idx > strings.LastIndex(repo, "/") {
		repo = repo[:idx]
	}
	idx := strings.LastIndex(repo, "/")
	if idx == -1 {
		return repo
	}
	return repo[idx+1:]
}

// imagePath extracts the repository path after the source registry host from a
// full image reference. If no registry host is detected, it returns the entire
// repository path.
// e.g. "registry.com/ns/app:1.0" → "ns/app", "library/nginx:latest" → "library/nginx"
func imagePath(ref string) string {
	ref = stripTag(ref)

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 1 {
		return ref
	}

	// If the first component contains a dot or colon, it's a registry host
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
		return parts[1]
	}
	return ref
}

// stripTag removes the tag from a full image reference.
func stripTag(ref string) string {
	if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
		return ref[:idx]
	}
	return ref
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
// Matching is done by short image name (last path component) to handle
// differences between bundle proxy paths and chart repository paths.
func (c *ChartMatcher) Match(name string) bool {
	shortName := imageName(name)
	for _, ref := range c.refs {
		if shortName == imageName(ref) {
			return true
		}
	}
	return false
}

// RemapChartValues walks the chart values structure, finds image references, and
// produces a remapped values map preserving the chart's original structure.
// Bundle images are matched by short image name (last path component of repository).
func RemapChartValues(chartValues map[string]interface{}, images []distribution.Image, registry, namespace string) map[string]interface{} {
	imageMap := make(map[string]distribution.Image)
	for _, img := range images {
		name := imageName(img.SourceRef)
		imageMap[name] = img
	}

	result := make(map[string]interface{})
	walkChart(chartValues, []string{}, result, imageMap, registry, namespace)
	return result
}

func walkChart(node interface{}, path []string, result map[string]interface{}, imageMap map[string]distribution.Image, registry, namespace string) {
	switch v := node.(type) {
	case map[string]interface{}:
		if isImageDefinition(v) {
			repo, _ := v["repository"].(string)
			name := imageName(repo)
			if img, ok := imageMap[name]; ok {
				remapped := remapImageDefinition(v, img, registry, namespace)
				setAtPath(result, path, remapped)
			} else {
				// Preserve unmapped image definitions as-is
				setAtPath(result, path, v)
			}
		} else {
			for k, child := range v {
				walkChart(child, append(path, k), result, imageMap, registry, namespace)
			}
		}
	case []interface{}:
		var remapped []interface{}
		matched := false
		for _, item := range v {
			if s, ok := item.(string); ok && looksLikeImageRef(s) {
				name := imageName(s)
				if img, ok := imageMap[name]; ok {
					ref := buildRemappedRef(img, registry, namespace)
					remapped = append(remapped, ref)
					matched = true
				} else {
					remapped = append(remapped, item)
				}
			} else {
				remapped = append(remapped, item)
			}
		}
		if matched {
			setAtPath(result, path, remapped)
		} else {
			// Preserve arrays that did not contain matched images
			setAtPath(result, path, v)
		}
	default:
		// Preserve scalar values at the current path
		if len(path) > 0 {
			setAtPath(result, path, v)
		}
	}
}

func isImageDefinition(m map[string]interface{}) bool {
	_, hasRegistry := m["registry"]
	_, hasRepository := m["repository"]
	_, hasTag := m["tag"]
	return hasRegistry && hasRepository && hasTag
}

func remapImageDefinition(m map[string]interface{}, img distribution.Image, registry, namespace string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	result["registry"] = registry
	result["tag"] = img.Tag

	path := imagePath(img.SourceRef)
	if namespace != "" {
		result["repository"] = namespace + "/" + path
	} else {
		result["repository"] = path
	}
	return result
}

func looksLikeImageRef(s string) bool {
	return strings.Contains(s, "/") && strings.Contains(s, ":")
}

func buildRemappedRef(img distribution.Image, registry, namespace string) string {
	path := imagePath(img.SourceRef)
	if namespace != "" {
		path = namespace + "/" + path
	}
	if registry != "" {
		return registry + "/" + path + ":" + img.Tag
	}
	return path + ":" + img.Tag
}

func setAtPath(root map[string]interface{}, path []string, value interface{}) {
	if len(path) == 0 {
		return
	}
	current := root
	for _, key := range path[:len(path)-1] {
		if _, ok := current[key]; !ok {
			current[key] = make(map[string]interface{})
		}
		next, ok := current[key].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

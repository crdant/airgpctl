package values

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/replicatedhq/airgapctl/pkg/distribution"
	"gopkg.in/yaml.v3"
)

func TestGenerator_Generate_SingleImage(t *testing.T) {
	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "myns",
		Template:  "{registry}/{namespace}/{name}:{tag}",
	}

	images := []distribution.Image{
		{
			SourceRef:  "nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	if err := g.Generate(images, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}

	want := "myregistry.example.com/myns/nginx:latest"
	if got := result["nginx"]; got != want {
		t.Errorf("nginx value = %q, want %q", got, want)
	}
}

func TestGenerator_Generate_MultipleImages(t *testing.T) {
	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "",
		Template:  "{registry}/{name}:{tag}",
	}

	images := []distribution.Image{
		{
			SourceRef:  "nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
		{
			SourceRef:  "postgres:14",
			Repository: "library/postgres",
			Tag:        "14",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	if err := g.Generate(images, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}

	if got := result["nginx"]; got != "myregistry.example.com/nginx:latest" {
		t.Errorf("nginx value = %q, want %q", got, "myregistry.example.com/nginx:latest")
	}
	if got := result["postgres"]; got != "myregistry.example.com/postgres:14" {
		t.Errorf("postgres value = %q, want %q", got, "myregistry.example.com/postgres:14")
	}
}

func TestGenerator_Generate_CustomTemplate(t *testing.T) {
	g := &Generator{
		Registry:  "reg.example.com",
		Namespace: "prod",
		Template:  "{registry}/{namespace}/{repository}:{tag}",
	}

	images := []distribution.Image{
		{
			SourceRef:  "registry.replicated.com/myapp/myimage:v1.0.0",
			Repository: "registry.replicated.com/myapp/myimage",
			Tag:        "v1.0.0",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	if err := g.Generate(images, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	want := "reg.example.com/prod/registry.replicated.com/myapp/myimage:v1.0.0"
	if got := result["myimage"]; got != want {
		t.Errorf("myimage value = %q, want %q", got, want)
	}
}

func TestGenerator_Generate_TagOverride(t *testing.T) {
	g := &Generator{
		Registry:    "myregistry.example.com",
		Namespace:   "myns",
		TagOverride: "stable",
		Template:    "{registry}/{namespace}/{name}:{tag}",
	}

	images := []distribution.Image{
		{
			SourceRef:  "nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	if err := g.Generate(images, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	want := "myregistry.example.com/myns/nginx:stable"
	if got := result["nginx"]; got != want {
		t.Errorf("nginx value = %q, want %q", got, want)
	}
}

func TestGenerator_Generate_DefaultTemplate(t *testing.T) {
	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "",
	}

	images := []distribution.Image{
		{
			SourceRef:  "nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	if err := g.Generate(images, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	// Default template should be {registry}/{namespace}/{name}:{tag} with empty namespace
	want := "myregistry.example.com/nginx:latest"
	if got := result["nginx"]; got != want {
		t.Errorf("nginx value = %q, want %q", got, want)
	}
}

func TestGenerator_Generate_WithFilter(t *testing.T) {
	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "",
		Template:  "{registry}/{name}:{tag}",
		Filter:    func(name string) bool { return name == "nginx" },
	}

	images := []distribution.Image{
		{
			SourceRef:  "nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
		{
			SourceRef:  "postgres:14",
			Repository: "library/postgres",
			Tag:        "14",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	if err := g.Generate(images, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}

	if _, ok := result["nginx"]; !ok {
		t.Errorf("expected nginx in result, got %v", result)
	}
	if _, ok := result["postgres"]; ok {
		t.Errorf("did not expect postgres in result, got %v", result)
	}
}

func TestGenerator_Generate_ChartAwarePartialMatch(t *testing.T) {
	// Simulate chart references: the chart references "nginx" and "redis"
	// The bundle contains "nginx:latest", "redis:alpine", "postgres:14"
	// Only nginx and redis should be included
	chartRefs := []string{"nginx", "redis"}
	matcher := NewChartMatcher(chartRefs)

	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "",
		Template:  "{registry}/{name}:{tag}",
		Filter:    matcher.Match,
	}

	images := []distribution.Image{
		{
			SourceRef:  "nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
		{
			SourceRef:  "redis:alpine",
			Repository: "library/redis",
			Tag:        "alpine",
		},
		{
			SourceRef:  "postgres:14",
			Repository: "library/postgres",
			Tag:        "14",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	if err := g.Generate(images, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}

	if _, ok := result["nginx"]; !ok {
		t.Errorf("expected nginx in result, got %v", result)
	}
	if _, ok := result["redis"]; !ok {
		t.Errorf("expected redis in result, got %v", result)
	}
	if _, ok := result["postgres"]; ok {
		t.Errorf("did not expect postgres in result, got %v", result)
	}
}

func TestGenerator_Generate_NoImages(t *testing.T) {
	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "",
		Template:  "{registry}/{name}:{tag}",
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	if err := g.Generate([]distribution.Image{}, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	// Should generate empty YAML or valid YAML with empty map
	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestGenerator_Generate_InvalidTemplate(t *testing.T) {
	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "",
		Template:  "{invalid}",
	}

	images := []distribution.Image{
		{
			SourceRef:  "nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	err := g.Generate(images, outputPath)
	if err == nil {
		t.Fatal("expected error for invalid template, got nil")
	}

	if !strings.Contains(err.Error(), "unknown template variable") {
		t.Errorf("expected 'unknown template variable' in error, got %v", err)
	}
}

func TestGenerator_Generate_DuplicateImageName(t *testing.T) {
	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "",
		Template:  "{registry}/{name}:{tag}",
	}

	images := []distribution.Image{
		{
			SourceRef:  "library/nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
		{
			SourceRef:  "bitnami/nginx:latest",
			Repository: "bitnami/nginx",
			Tag:        "latest",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "images.yaml")

	err := g.Generate(images, outputPath)
	if err == nil {
		t.Fatal("expected error for duplicate image name, got nil")
	}

	if !strings.Contains(err.Error(), "duplicate image name") {
		t.Errorf("expected 'duplicate image name' in error, got %v", err)
	}
}

func TestChartMatcher(t *testing.T) {
	matcher := NewChartMatcher([]string{"nginx", "redis"})

	tests := []struct {
		name string
		want bool
	}{
		{"nginx", true},
		{"redis", true},
		{"library/nginx", true},   // short name matches chart ref "nginx"
		{"docker.io/library/nginx", true}, // short name matches chart ref "nginx"
		{"nginx-plus", false},     // substring mismatch — no false positive
		{"my-nginx", false},       // substring mismatch — no false positive
		{"postgres", false},
		{"predis", false},         // substring mismatch — no false positive
	}

	for _, tt := range tests {
		got := matcher.Match(tt.name)
		if got != tt.want {
			t.Errorf("ChartMatcher.Match(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGenerator_Generate_OutputFileCreated(t *testing.T) {
	g := &Generator{
		Registry:  "myregistry.example.com",
		Namespace: "",
		Template:  "{registry}/{name}:{tag}",
	}

	images := []distribution.Image{
		{
			SourceRef:  "nginx:latest",
			Repository: "library/nginx",
			Tag:        "latest",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "sub", "dir", "custom.yaml")

	if err := g.Generate(images, outputPath); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("expected output file to be created at %s", outputPath)
	}
}

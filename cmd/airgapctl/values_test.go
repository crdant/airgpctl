package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExtractImageRefs_NoFalsePositives(t *testing.T) {
	input := `
some_repository: foo
myregistry: bar
imagePullPolicy: Always
`
	got := extractImageRefs(input)
	if len(got) != 0 {
		t.Errorf("expected no refs, got %v", got)
	}
}

func TestExtractImageRefs_RepositoryKey(t *testing.T) {
	input := `
repository: nginx
  repository: nginx
    repository: nginx
`
	got := extractImageRefs(input)
	want := []string{"nginx", "nginx", "nginx"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestExtractImageRefs_RepositoryKeyQuoted(t *testing.T) {
	input := `repository: "nginx"`
	got := extractImageRefs(input)
	want := []string{"nginx"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestExtractImageRefs_ImageKey(t *testing.T) {
	input := `image: "docker.io/nginx:latest"`
	got := extractImageRefs(input)
	want := []string{"docker.io/nginx:latest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestExtractImageRefs_ImageKeySingleQuoted(t *testing.T) {
	input := `image: 'docker.io/nginx:latest'`
	got := extractImageRefs(input)
	want := []string{"docker.io/nginx:latest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestExtractImageRefs_ImageKeyUnquoted(t *testing.T) {
	input := `image: docker.io/nginx:latest`
	got := extractImageRefs(input)
	want := []string{"docker.io/nginx:latest"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestExtractImageRefs_ArrayItems(t *testing.T) {
	input := `
releaseImages:
  - docker.io/nginx:latest
  - myregistry.com/app:1.0
  - nginx:alpine
`
	got := extractImageRefs(input)
	want := []string{"docker.io/nginx:latest", "myregistry.com/app:1.0", "nginx:alpine"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestExtractImageRefs_EmptyValues(t *testing.T) {
	input := `
repository:
image:
  - ""
`
	got := extractImageRefs(input)
	if len(got) != 0 {
		t.Errorf("expected no refs, got %v", got)
	}
}

func TestIsChartTarball(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"chart.tgz", true},
		{"chart.tar.gz", true},
		{"chart.TGZ", true},
		{"chart.yaml", false},
		{"chart.gz", false},
		{"/path/to/chart.tgz", true},
		{"/path/to/file.gz", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isChartTarball(tt.path)
			if got != tt.want {
				t.Errorf("isChartTarball(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractTarball_DirectoryTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "evil.tgz")

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{
		Name: "../../../etc/passwd",
		Size: 5,
		Mode: 0644,
	})
	_, _ = tw.Write([]byte("hello"))
	_ = tw.Close()
	_ = gw.Close()
	_ = os.WriteFile(tarPath, buf.Bytes(), 0644)

	dst := filepath.Join(tmpDir, "extract")
	_ = os.MkdirAll(dst, 0755)
	if err := extractTarball(tarPath, dst); err == nil {
		t.Fatal("expected error for directory traversal, got nil")
	}
}

func TestWriteValuesYAML(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "subdir", "values.yaml")
	values := map[string]interface{}{
		"image": map[string]interface{}{
			"registry":   "myreg.io",
			"repository": "myapp",
			"tag":        "1.0",
		},
	}

	if err := writeValuesYAML(values, outPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty output file")
	}
}

// --- findChartDir tests ---

func TestFindChartDir_Match(t *testing.T) {
	tmpDir := t.TempDir()

	// Create foo/ with Chart.yaml name: foo
	fooDir := filepath.Join(tmpDir, "foo")
	if err := os.MkdirAll(fooDir, 0755); err != nil {
		t.Fatalf("creating foo dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fooDir, "Chart.yaml"), []byte("name: foo\n"), 0644); err != nil {
		t.Fatalf("writing foo Chart.yaml: %v", err)
	}

	// Create bar/ with Chart.yaml name: bar
	barDir := filepath.Join(tmpDir, "bar")
	if err := os.MkdirAll(barDir, 0755); err != nil {
		t.Fatalf("creating bar dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(barDir, "Chart.yaml"), []byte("name: bar\n"), 0644); err != nil {
		t.Fatalf("writing bar Chart.yaml: %v", err)
	}

	got, err := findChartDir(tmpDir, "bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := barDir
	if got != want {
		t.Errorf("findChartDir(%q, %q) = %q, want %q", tmpDir, "bar", got, want)
	}
}

func TestFindChartDir_Fallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create foo/ with Chart.yaml name: foo
	fooDir := filepath.Join(tmpDir, "foo")
	if err := os.MkdirAll(fooDir, 0755); err != nil {
		t.Fatalf("creating foo dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fooDir, "Chart.yaml"), []byte("name: foo\n"), 0644); err != nil {
		t.Fatalf("writing foo Chart.yaml: %v", err)
	}

	// Create bar/ with Chart.yaml name: baz (does not match expected)
	barDir := filepath.Join(tmpDir, "bar")
	if err := os.MkdirAll(barDir, 0755); err != nil {
		t.Fatalf("creating bar dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(barDir, "Chart.yaml"), []byte("name: baz\n"), 0644); err != nil {
		t.Fatalf("writing bar Chart.yaml: %v", err)
	}

	got, err := findChartDir(tmpDir, "nomatch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Fallback returns the first directory found; filesystem ordering is
	// undefined, so accept either directory.
	if got != fooDir && got != barDir {
		t.Errorf("findChartDir(%q, %q) = %q, want either %q or %q", tmpDir, "nomatch", got, fooDir, barDir)
	}
}

func TestFindChartDir_SingleDir(t *testing.T) {
	tmpDir := t.TempDir()

	chartDir := filepath.Join(tmpDir, "chart")
	if err := os.MkdirAll(chartDir, 0755); err != nil {
		t.Fatalf("creating chart dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("name: chart\n"), 0644); err != nil {
		t.Fatalf("writing chart Chart.yaml: %v", err)
	}

	got, err := findChartDir(tmpDir, "chart")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := chartDir
	if got != want {
		t.Errorf("findChartDir(%q, %q) = %q, want %q", tmpDir, "chart", got, want)
	}
}

func TestFindChartDir_NoDirs(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := findChartDir(tmpDir, "missing")
	if err == nil {
		t.Fatal("expected error when no chart directories exist, got nil")
	}
}

func TestFindChartDir_MissingChartYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory with no Chart.yaml
	chartDir := filepath.Join(tmpDir, "chart")
	if err := os.MkdirAll(chartDir, 0755); err != nil {
		t.Fatalf("creating chart dir: %v", err)
	}

	got, err := findChartDir(tmpDir, "chart")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := chartDir
	if got != want {
		t.Errorf("findChartDir(%q, %q) = %q, want %q", tmpDir, "chart", got, want)
	}
}

// --- extractTarball tests ---

func TestExtractTarball_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "test.tgz")

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add a directory
	if err := tw.WriteHeader(&tar.Header{
		Name:     "subdir/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	}); err != nil {
		t.Fatalf("writing tar dir header: %v", err)
	}

	// Add a file
	content := []byte("hello world")
	if err := tw.WriteHeader(&tar.Header{
		Name: "subdir/file.txt",
		Size: int64(len(content)),
		Mode: 0644,
	}); err != nil {
		t.Fatalf("writing tar file header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("writing tar file content: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}
	if err := os.WriteFile(tarPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("writing tar file: %v", err)
	}

	dst := filepath.Join(tmpDir, "extract")
	if err := extractTarball(tarPath, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dst, "subdir", "file.txt"))
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("extracted content = %q, want %q", string(data), "hello world")
	}
}

func TestExtractTarball_NonExistentFile(t *testing.T) {
	dst := t.TempDir()
	if err := extractTarball("/nonexistent/path/to/file.tgz", dst); err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestExtractTarball_InvalidGzip(t *testing.T) {
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "invalid.tgz")
	if err := os.WriteFile(tarPath, []byte("not valid gzip data"), 0644); err != nil {
		t.Fatalf("writing invalid gzip file: %v", err)
	}

	dst := filepath.Join(tmpDir, "extract")
	if err := extractTarball(tarPath, dst); err == nil {
		t.Fatal("expected error for invalid gzip data, got nil")
	}
}

// --- writeValuesYAML tests ---

func TestWriteValuesYAML_ValidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "values.yaml")
	values := map[string]interface{}{
		"image": map[string]interface{}{
			"registry":   "myreg.io",
			"repository": "myapp",
			"tag":        "1.0",
		},
	}

	if err := writeValuesYAML(values, outPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}

	img, ok := parsed["image"].(map[string]interface{})
	if !ok {
		t.Fatal("expected image key in parsed YAML")
	}
	if img["registry"] != "myreg.io" {
		t.Errorf("registry = %v, want myreg.io", img["registry"])
	}
	if img["repository"] != "myapp" {
		t.Errorf("repository = %v, want myapp", img["repository"])
	}
	if img["tag"] != "1.0" {
		t.Errorf("tag = %v, want 1.0", img["tag"])
	}
}

func TestWriteValuesYAML_UnmarshalableValue(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "values.yaml")
	values := map[string]interface{}{
		"bad": make(chan int),
	}

	if err := writeValuesYAML(values, outPath); err == nil {
		t.Fatal("expected error for unmarshalable value, got nil")
	}
}

func TestWriteValuesYAML_CreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "a", "b", "c", "values.yaml")
	values := map[string]interface{}{
		"key": "value",
	}

	if err := writeValuesYAML(values, outPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Fatalf("expected output file to be created at %s", outPath)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "a", "b", "c")); os.IsNotExist(err) {
		t.Fatal("expected parent directories to be created")
	}
}

// --- chartNameFromTarball tests ---

func TestChartNameFromTarball(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"chart.tgz", "chart"},
		{"chart.tar.gz", "chart"},
		{"chart.TGZ", "chart"},
		{"chart.TAR.GZ", "chart"},
		{"/path/to/chart.tgz", "chart"},
		{"/path/to/my-chart-1.2.3.tar.gz", "my-chart-1.2.3"},
		{"chart.yaml", "chart.yaml"},
		{"chart", "chart"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := chartNameFromTarball(tt.path)
			if got != tt.want {
				t.Errorf("chartNameFromTarball(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

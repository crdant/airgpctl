package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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

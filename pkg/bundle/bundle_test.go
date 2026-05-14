package bundle

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func createMockAirgapBundle(t *testing.T, dir string, airgapYamlContent string) string {
	t.Helper()
	bundlePath := filepath.Join(dir, "test.airgap")
	f, err := os.Create(bundlePath)
	if err != nil {
		t.Fatalf("creating mock bundle: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	w := tar.NewWriter(gw)
	defer w.Close()

	header := &tar.Header{
		Name: "airgap.yaml",
		Size: int64(len(airgapYamlContent)),
		Mode: 0644,
	}
	if err := w.WriteHeader(header); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := w.Write([]byte(airgapYamlContent)); err != nil {
		t.Fatalf("writing tar content: %v", err)
	}

	return bundlePath
}

func createPlainTarBundle(t *testing.T, dir string, airgapYamlContent string) string {
	t.Helper()
	bundlePath := filepath.Join(dir, "test.airgap")
	f, err := os.Create(bundlePath)
	if err != nil {
		t.Fatalf("creating mock bundle: %v", err)
	}
	defer f.Close()

	w := tar.NewWriter(f)
	defer w.Close()

	header := &tar.Header{
		Name: "airgap.yaml",
		Size: int64(len(airgapYamlContent)),
		Mode: 0644,
	}
	if err := w.WriteHeader(header); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := w.Write([]byte(airgapYamlContent)); err != nil {
		t.Fatalf("writing tar content: %v", err)
	}

	return bundlePath
}

func TestOpenBundle(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantImages []string
		wantErr    bool
	}{
		{
			name: "valid bundle with multiple images",
			content: `spec:
  savedImages:
    - "nginx:latest"
    - "postgres:14"
    - "registry.replicated.com/myapp/myimage:v1.0.0"
`,
			wantImages: []string{
				"nginx:latest",
				"postgres:14",
				"registry.replicated.com/myapp/myimage:v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "valid bundle with single image",
			content: `spec:
  savedImages:
    - "busybox:latest"
`,
			wantImages: []string{"busybox:latest"},
			wantErr:    false,
		},
		{
			name: "valid bundle with no images",
			content: `spec:
  savedImages: []
`,
			wantImages: []string{},
			wantErr:    false,
		},
		{
			name:     "missing airgap.yaml in bundle",
			content:  "", // Will create empty tar
			wantImages: nil,
			wantErr:    true,
		},
		{
			name: "missing SavedImages field",
			content: `spec:
  otherField: "value"
`,
			wantImages: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var bundlePath string
			if tt.name == "missing airgap.yaml in bundle" {
				// Create an empty gzipped tar
				bundlePath = filepath.Join(tmpDir, "empty.airgap")
				f, err := os.Create(bundlePath)
				if err != nil {
					t.Fatalf("creating empty bundle: %v", err)
				}
				gw := gzip.NewWriter(f)
				w := tar.NewWriter(gw)
				w.Close()
				gw.Close()
				f.Close()
			} else {
				bundlePath = createMockAirgapBundle(t, tmpDir, tt.content)
			}

			b, err := OpenBundle(bundlePath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("OpenBundle() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if len(b.Spec.SavedImages) != len(tt.wantImages) {
				t.Errorf("SavedImages length = %d, want %d", len(b.Spec.SavedImages), len(tt.wantImages))
			} else {
				for i, img := range tt.wantImages {
					if b.Spec.SavedImages[i] != img {
						t.Errorf("SavedImages[%d] = %q, want %q", i, b.Spec.SavedImages[i], img)
					}
				}
			}
		})
	}
}

func TestOpenBundle_FileNotFound(t *testing.T) {
	_, err := OpenBundle("/nonexistent/path/bundle.airgap")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestBundle_ExtractTo(t *testing.T) {
	content := `spec:
  savedImages:
    - "nginx:latest"
`
	tmpDir := t.TempDir()
	bundlePath := createMockAirgapBundle(t, tmpDir, content)

	b, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatalf("OpenBundle() error = %v", err)
	}

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := b.ExtractTo(extractDir); err != nil {
		t.Fatalf("ExtractTo() error = %v", err)
	}

	// Verify airgap.yaml was extracted
	yamlPath := filepath.Join(extractDir, "airgap.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Fatalf("expected airgap.yaml to be extracted, but it was not found")
	}

	// Verify content
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("reading extracted airgap.yaml: %v", err)
	}
	if string(data) != content {
		t.Errorf("extracted content mismatch\ngot:\n%s\nwant:\n%s", string(data), content)
	}
}

func TestBundle_ExtractTo_DirectoryTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "evil.airgap")
		f, err := os.Create(bundlePath)
	if err != nil {
		t.Fatalf("creating evil bundle: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	w := tar.NewWriter(gw)
	defer w.Close()

	// Add a malicious entry that tries to escape the extraction directory
	evilContent := "evil content"
	header := &tar.Header{
		Name: "../../evil.txt",
		Size: int64(len(evilContent)),
		Mode: 0644,
	}
	if err := w.WriteHeader(header); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := w.Write([]byte(evilContent)); err != nil {
		t.Fatalf("writing tar content: %v", err)
	}

	b := &Bundle{bundlePath: bundlePath}
	extractDir := filepath.Join(tmpDir, "extracted")
	err = b.ExtractTo(extractDir)
	if err == nil {
		t.Fatal("expected error for directory traversal attempt, got nil")
	}

	// Ensure the evil file was NOT created outside the extraction directory
	evilPath := filepath.Join(tmpDir, "evil.txt")
	if _, err := os.Stat(evilPath); !os.IsNotExist(err) {
		t.Fatalf("evil file was created outside extraction directory: %s", evilPath)
	}
}

func TestOpenBundle_PlainTar(t *testing.T) {
	content := `spec:
  savedImages:
    - "nginx:latest"
`
	tmpDir := t.TempDir()
	bundlePath := createPlainTarBundle(t, tmpDir, content)

	b, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatalf("OpenBundle() error = %v", err)
	}

	if len(b.Spec.SavedImages) != 1 || b.Spec.SavedImages[0] != "nginx:latest" {
		t.Errorf("unexpected images: %v", b.Spec.SavedImages)
	}
}

func TestBundle_ExtractTo_PlainTar(t *testing.T) {
	content := `spec:
  savedImages:
    - "nginx:latest"
`
	tmpDir := t.TempDir()
	bundlePath := createPlainTarBundle(t, tmpDir, content)

	b, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatalf("OpenBundle() error = %v", err)
	}

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := b.ExtractTo(extractDir); err != nil {
		t.Fatalf("ExtractTo() error = %v", err)
	}

	yamlPath := filepath.Join(extractDir, "airgap.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Fatalf("expected airgap.yaml to be extracted, but it was not found")
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("reading extracted airgap.yaml: %v", err)
	}
	if string(data) != content {
		t.Errorf("extracted content mismatch\ngot:\n%s\nwant:\n%s", string(data), content)
	}
}

func TestOpenBundle_GzippedTar(t *testing.T) {
	content := `spec:
  savedImages:
    - "postgres:14"
`
	tmpDir := t.TempDir()
	bundlePath := createMockAirgapBundle(t, tmpDir, content)

	b, err := OpenBundle(bundlePath)
	if err != nil {
		t.Fatalf("OpenBundle() error = %v", err)
	}

	if len(b.Spec.SavedImages) != 1 || b.Spec.SavedImages[0] != "postgres:14" {
		t.Errorf("unexpected images: %v", b.Spec.SavedImages)
	}
}

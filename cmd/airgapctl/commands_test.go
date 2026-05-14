package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createMockAirgapBundle creates a .airgap tar containing airgap.yaml and a minimal
// Docker distribution v2 layout for the specified images.
func createMockAirgapBundle(t *testing.T, dir string, images []string, layouts []mockImageLayout) string {
	t.Helper()

	bundlePath := filepath.Join(dir, "test.airgap")
	f, err := os.Create(bundlePath)
	if err != nil {
		t.Fatalf("creating mock bundle: %v", err)
	}
	defer f.Close()

	w := tar.NewWriter(f)
	defer w.Close()

	// Write airgap.yaml
	yamlContent := fmt.Sprintf(`Version: "1"
Type: "airgap"
SavedImages:
%s
`, formatSavedImages(images))
	writeTarFile(t, w, "airgap.yaml", []byte(yamlContent))

	// Write distribution layout for each image
	for _, layout := range layouts {
		writeDistributionLayout(t, w, layout)
	}

	// Write shared blob data for the hardcoded digest used by makeSingleArchManifest
	blobPath := "blobs/sha256/e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855/data"
	writeTarFile(t, w, blobPath, makeBlobData())

	return bundlePath
}

type mockImageLayout struct {
	repo           string
	tag            string
	manifestDigest string
	manifestJSON   []byte
}

func formatSavedImages(images []string) string {
	var b bytes.Buffer
	for _, img := range images {
		fmt.Fprintf(&b, "  - %q\n", img)
	}
	return b.String()
}

func writeTarFile(t *testing.T, w *tar.Writer, name string, data []byte) {
	t.Helper()
	header := &tar.Header{
		Name: name,
		Size: int64(len(data)),
		Mode: 0644,
	}
	if err := w.WriteHeader(header); err != nil {
		t.Fatalf("writing tar header for %s: %v", name, err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("writing tar content for %s: %v", name, err)
	}
}

func writeDistributionLayout(t *testing.T, w *tar.Writer, layout mockImageLayout) {
	t.Helper()

	// Manifest revision link
	revPath := fmt.Sprintf("images/%s/_manifests/revisions/sha256/%s/link", layout.repo, layout.manifestDigest)
	writeTarFile(t, w, revPath, []byte("sha256:"+layout.manifestDigest))

	// Tag current link
	tagPath := fmt.Sprintf("images/%s/_manifests/tags/%s/current/link", layout.repo, layout.tag)
	writeTarFile(t, w, tagPath, []byte("sha256:"+layout.manifestDigest))

	// Blob data
	blobPath := fmt.Sprintf("blobs/sha256/%s/data", layout.manifestDigest)
	writeTarFile(t, w, blobPath, layout.manifestJSON)
}

func makeSingleArchManifest(t *testing.T) []byte {
	t.Helper()
	m := map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]interface{}{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size":      7023,
			"digest":    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		"layers": []interface{}{
			map[string]interface{}{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size":      32654,
				"digest":    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			},
		},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshaling manifest: %v", err)
	}
	return data
}

// makeBlobData returns dummy blob data for the hardcoded digests in makeSingleArchManifest.
func makeBlobData() []byte {
	return []byte("dummy blob data")
}

func TestListCommand(t *testing.T) {
	tmpDir := t.TempDir()
	manifestJSON := makeSingleArchManifest(t)
	bundlePath := createMockAirgapBundle(t, tmpDir, []string{"nginx:latest"}, []mockImageLayout{
		{repo: "library/nginx", tag: "latest", manifestDigest: "nginx123", manifestJSON: manifestJSON},
	})

	root := newRootCmd()
	root.SetArgs([]string{"list", "--bundle", bundlePath})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("list command failed: %v\noutput: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "nginx:latest") {
		t.Errorf("expected output to contain 'nginx:latest', got:\n%s", output)
	}
	if !strings.Contains(output, "sha256:nginx123") {
		t.Errorf("expected output to contain manifest digest, got:\n%s", output)
	}
}

func TestListCommand_MultiArch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create child manifests
	amd64JSON := makeSingleArchManifest(t)
	arm64JSON := makeSingleArchManifest(t)

	// Create manifest list
	listJSON, err := json.Marshal(map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests": []interface{}{
			map[string]interface{}{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"size":      len(amd64JSON),
				"digest":    "sha256:amd64digest",
				"platform": map[string]string{
					"architecture": "amd64",
					"os":           "linux",
				},
			},
			map[string]interface{}{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"size":      len(arm64JSON),
				"digest":    "sha256:arm64digest",
				"platform": map[string]string{
					"architecture": "arm64",
					"os":           "linux",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshaling manifest list: %v", err)
	}

	bundlePath := createMockAirgapBundle(t, tmpDir, []string{"nginx:latest"}, []mockImageLayout{
		{repo: "library/nginx", tag: "latest", manifestDigest: "listdigest", manifestJSON: listJSON},
	})

	root := newRootCmd()
	root.SetArgs([]string{"list", "--bundle", bundlePath})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("list command failed: %v\noutput: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "nginx:latest") {
		t.Errorf("expected output to contain 'nginx:latest', got:\n%s", output)
	}
	if !strings.Contains(output, "multi-arch") {
		t.Errorf("expected output to indicate multi-arch, got:\n%s", output)
	}
}

func TestValuesCommand(t *testing.T) {
	tmpDir := t.TempDir()
	manifestJSON := makeSingleArchManifest(t)

	bundlePath := createMockAirgapBundle(t, tmpDir, []string{"nginx:latest"}, []mockImageLayout{
		{repo: "library/nginx", tag: "latest", manifestDigest: "nginx123", manifestJSON: manifestJSON},
	})

	// Create a minimal chart directory
	chartDir := filepath.Join(tmpDir, "chart")
	if err := os.MkdirAll(chartDir, 0755); err != nil {
		t.Fatalf("creating chart dir: %v", err)
	}
	chartValues := `image:
  repository: nginx
  tag: latest
`
	if err := os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(chartValues), 0644); err != nil {
		t.Fatalf("writing chart values.yaml: %v", err)
	}

	outputPath := filepath.Join(tmpDir, "output.yaml")

	root := newRootCmd()
	root.SetArgs([]string{
		"values",
		"--bundle", bundlePath,
		"--chart", chartDir,
		"--registry", "myregistry.example.com",
		"--namespace", "myapp",
		"--output", outputPath,
	})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("values command failed: %v\noutput: %s", err, out.String())
	}

	// Verify output file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("expected output file %s to exist", outputPath)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	if !strings.Contains(string(data), "myregistry.example.com") {
		t.Errorf("expected output to contain destination registry, got:\n%s", string(data))
	}
}

func TestPushCommand(t *testing.T) {
	// Create a mock registry server
	uploaded := make(map[string]bool)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate registry v2 API
		switch {
		case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/blobs/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/blobs/uploads/"):
			w.Header().Set("Location", "/upload")
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/upload"):
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/"):
			uploaded[r.URL.Path] = true
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	manifestJSON := makeSingleArchManifest(t)
	bundlePath := createMockAirgapBundle(t, tmpDir, []string{"nginx:latest"}, []mockImageLayout{
		{repo: "library/nginx", tag: "latest", manifestDigest: "nginx123", manifestJSON: manifestJSON},
	})

	root := newRootCmd()
	root.SetArgs([]string{
		"push",
		"--bundle", bundlePath,
		"--registry", server.URL,
		"--username", "testuser",
		"--password", "testpass",
		"--tls-skip-verify",
	})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("push command failed: %v\noutput: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "Pushing") {
		t.Errorf("expected output to contain 'Pushing', got:\n%s", output)
	}
	if !strings.Contains(output, "1 pushed") {
		t.Errorf("expected output to show pushed count, got:\n%s", output)
	}
}

package distribution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// createMockDistributionLayout creates a minimal Docker distribution v2 on-disk layout
// in the given directory with the specified repositories, tags, and manifest content.
func createMockDistributionLayout(t *testing.T, dir string, repo string, tag string, manifestDigest string, manifestJSON []byte) {
	t.Helper()

	// Create manifest link files
	manifestDir := filepath.Join(dir, "images", repo, "_manifests", "revisions", "sha256", manifestDigest)
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatalf("creating manifest dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "link"), []byte("sha256:"+manifestDigest), 0644); err != nil {
		t.Fatalf("writing manifest revision link: %v", err)
	}

	tagDir := filepath.Join(dir, "images", repo, "_manifests", "tags", tag, "current")
	if err := os.MkdirAll(tagDir, 0755); err != nil {
		t.Fatalf("creating tag dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tagDir, "link"), []byte("sha256:"+manifestDigest), 0644); err != nil {
		t.Fatalf("writing tag link: %v", err)
	}

	// Create blob data
	blobDir := filepath.Join(dir, "blobs", "sha256", manifestDigest)
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		t.Fatalf("creating blob dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blobDir, "data"), manifestJSON, 0644); err != nil {
		t.Fatalf("writing blob data: %v", err)
	}
}

// makeSingleArchManifest creates a Docker v2 manifest JSON
func makeSingleArchManifest(t *testing.T, mediaType string) []byte {
	t.Helper()
	m := map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     mediaType,
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

// makeManifestList creates a Docker manifest list / OCI index JSON
func makeManifestList(t *testing.T, manifests []map[string]interface{}) []byte {
	t.Helper()
	m := map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests":     manifests,
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshaling manifest list: %v", err)
	}
	return data
}

func TestNewWalker(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		w := NewWalker(tmpDir)
		if w == nil {
			t.Fatal("expected walker, got nil")
		}
		if w.basePath != tmpDir {
			t.Errorf("basePath = %q, want %q", w.basePath, tmpDir)
		}
	})
}

func TestWalker_ResolveImages_SingleArch(t *testing.T) {
	tmpDir := t.TempDir()

	manifestDigest := "abc123"
	manifestJSON := makeSingleArchManifest(t, "application/vnd.docker.distribution.manifest.v2+json")
	createMockDistributionLayout(t, tmpDir, "library/nginx", "latest", manifestDigest, manifestJSON)

	w := NewWalker(tmpDir)
	images, err := w.ResolveImages([]string{"nginx:latest"})
	if err != nil {
		t.Fatalf("ResolveImages error: %v", err)
	}

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}

	img := images[0]
	if img.SourceRef != "nginx:latest" {
		t.Errorf("SourceRef = %q, want %q", img.SourceRef, "nginx:latest")
	}
	if img.Repository != "library/nginx" {
		t.Errorf("Repository = %q, want %q", img.Repository, "library/nginx")
	}
	if img.Tag != "latest" {
		t.Errorf("Tag = %q, want %q", img.Tag, "latest")
	}
	if img.Digest != "sha256:"+manifestDigest {
		t.Errorf("Digest = %q, want %q", img.Digest, "sha256:"+manifestDigest)
	}
	if img.IsMultiArch {
		t.Error("expected IsMultiArch = false for single-arch manifest")
	}
	if len(img.Manifests) != 0 {
		t.Errorf("expected 0 platform manifests, got %d", len(img.Manifests))
	}
}

func TestWalker_ResolveImages_MultiArch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the child manifests first
	amd64Digest := "amd64manifest"
	amd64JSON := makeSingleArchManifest(t, "application/vnd.docker.distribution.manifest.v2+json")
	createMockDistributionLayout(t, tmpDir, "library/nginx", "latest-amd64", amd64Digest, amd64JSON)

	arm64Digest := "arm64manifest"
	arm64JSON := makeSingleArchManifest(t, "application/vnd.docker.distribution.manifest.v2+json")
	createMockDistributionLayout(t, tmpDir, "library/nginx", "latest-arm64", arm64Digest, arm64JSON)

	// Create the manifest list
	listDigest := "listdigest"
	listJSON := makeManifestList(t, []map[string]interface{}{
		{
			"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
			"size":      len(amd64JSON),
			"digest":    "sha256:" + amd64Digest,
			"platform": map[string]string{
				"architecture": "amd64",
				"os":           "linux",
			},
		},
		{
			"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
			"size":      len(arm64JSON),
			"digest":    "sha256:" + arm64Digest,
			"platform": map[string]string{
				"architecture": "arm64",
				"os":           "linux",
			},
		},
	})
	createMockDistributionLayout(t, tmpDir, "library/nginx", "latest", listDigest, listJSON)

	w := NewWalker(tmpDir)
	images, err := w.ResolveImages([]string{"nginx:latest"})
	if err != nil {
		t.Fatalf("ResolveImages error: %v", err)
	}

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}

	img := images[0]
	if !img.IsMultiArch {
		t.Error("expected IsMultiArch = true for manifest list")
	}
	if len(img.Manifests) != 2 {
		t.Fatalf("expected 2 platform manifests, got %d", len(img.Manifests))
	}

	// Check first platform manifest
	if img.Manifests[0].Digest != "sha256:"+amd64Digest {
		t.Errorf("Manifests[0].Digest = %q, want %q", img.Manifests[0].Digest, "sha256:"+amd64Digest)
	}
	if img.Manifests[0].Platform != "linux/amd64" {
		t.Errorf("Manifests[0].Platform = %q, want %q", img.Manifests[0].Platform, "linux/amd64")
	}

	// Check second platform manifest
	if img.Manifests[1].Digest != "sha256:"+arm64Digest {
		t.Errorf("Manifests[1].Digest = %q, want %q", img.Manifests[1].Digest, "sha256:"+arm64Digest)
	}
	if img.Manifests[1].Platform != "linux/arm64" {
		t.Errorf("Manifests[1].Platform = %q, want %q", img.Manifests[1].Platform, "linux/arm64")
	}
}

func TestWalker_ResolveImages_MultipleRepos(t *testing.T) {
	tmpDir := t.TempDir()

	nginxDigest := "nginx123"
	nginxJSON := makeSingleArchManifest(t, "application/vnd.docker.distribution.manifest.v2+json")
	createMockDistributionLayout(t, tmpDir, "library/nginx", "latest", nginxDigest, nginxJSON)

	postgresDigest := "pg123"
	postgresJSON := makeSingleArchManifest(t, "application/vnd.docker.distribution.manifest.v2+json")
	createMockDistributionLayout(t, tmpDir, "library/postgres", "14", postgresDigest, postgresJSON)

	w := NewWalker(tmpDir)
	images, err := w.ResolveImages([]string{"nginx:latest", "postgres:14"})
	if err != nil {
		t.Fatalf("ResolveImages error: %v", err)
	}

	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}

	found := make(map[string]string)
	for _, img := range images {
		found[img.SourceRef] = img.Digest
	}

	if d, ok := found["nginx:latest"]; !ok || d != "sha256:"+nginxDigest {
		t.Errorf("nginx:latest digest = %q, want %q", d, "sha256:"+nginxDigest)
	}
	if d, ok := found["postgres:14"]; !ok || d != "sha256:"+postgresDigest {
		t.Errorf("postgres:14 digest = %q, want %q", d, "sha256:"+postgresDigest)
	}
}

func TestWalker_ResolveImages_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	w := NewWalker(tmpDir)
	images, err := w.ResolveImages([]string{"nonexistent:latest"})
	if err == nil {
		t.Fatal("expected error for nonexistent image, got nil")
	}
	if images != nil {
		t.Errorf("expected nil images on error, got %v", images)
	}
}

func TestWalker_ResolveImages_PartialMatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Only create nginx, not postgres
	nginxDigest := "nginx123"
	nginxJSON := makeSingleArchManifest(t, "application/vnd.docker.distribution.manifest.v2+json")
	createMockDistributionLayout(t, tmpDir, "library/nginx", "latest", nginxDigest, nginxJSON)

	w := NewWalker(tmpDir)
	_, err := w.ResolveImages([]string{"nginx:latest", "postgres:14"})
	if err == nil {
		t.Fatal("expected error when some images not found")
	}
}

func TestWalker_ResolveImages_FullyQualifiedRegistry(t *testing.T) {
	tmpDir := t.TempDir()

	manifestDigest := "fq123"
	manifestJSON := makeSingleArchManifest(t, "application/vnd.docker.distribution.manifest.v2+json")
	createMockDistributionLayout(t, tmpDir, "registry.replicated.com/myapp/myimage", "v1.0.0", manifestDigest, manifestJSON)

	w := NewWalker(tmpDir)
	images, err := w.ResolveImages([]string{"registry.replicated.com/myapp/myimage:v1.0.0"})
	if err != nil {
		t.Fatalf("ResolveImages error: %v", err)
	}

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}

	img := images[0]
	if img.Repository != "registry.replicated.com/myapp/myimage" {
		t.Errorf("Repository = %q, want %q", img.Repository, "registry.replicated.com/myapp/myimage")
	}
	if img.Tag != "v1.0.0" {
		t.Errorf("Tag = %q, want %q", img.Tag, "v1.0.0")
	}
}

func TestWalker_ResolveImages_MissingTag(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repo but no tag
	if err := os.MkdirAll(filepath.Join(tmpDir, "images", "library", "nginx", "_manifests", "tags"), 0755); err != nil {
		t.Fatalf("creating tag dir: %v", err)
	}

	w := NewWalker(tmpDir)
	_, err := w.ResolveImages([]string{"nginx:latest"})
	if err == nil {
		t.Fatal("expected error for missing tag")
	}
}

func TestWalker_ReadBlob(t *testing.T) {
	tmpDir := t.TempDir()

	manifestDigest := "readblob123"
	manifestJSON := makeSingleArchManifest(t, "application/vnd.docker.distribution.manifest.v2+json")
	createMockDistributionLayout(t, tmpDir, "library/nginx", "latest", manifestDigest, manifestJSON)

	w := NewWalker(tmpDir)
	data, err := w.ReadBlob("sha256:" + manifestDigest)
	if err != nil {
		t.Fatalf("ReadBlob error: %v", err)
	}

	if string(data) != string(manifestJSON) {
		t.Errorf("ReadBlob returned wrong content")
	}
}

func TestManifestBlobs(t *testing.T) {
	manifest := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": 7023,
			"digest": "sha256:configdigest"
		},
		"layers": [
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size": 32654,
				"digest": "sha256:layer1digest"
			},
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size": 16724,
				"digest": "sha256:layer2digest"
			}
		]
	}`)

	digests, err := ManifestBlobs(manifest)
	if err != nil {
		t.Fatalf("ManifestBlobs error: %v", err)
	}

	want := []string{"sha256:configdigest", "sha256:layer1digest", "sha256:layer2digest"}
	if len(digests) != len(want) {
		t.Fatalf("expected %d blobs, got %d", len(want), len(digests))
	}
	for i, d := range want {
		if digests[i] != d {
			t.Errorf("blob[%d] = %q, want %q", i, digests[i], d)
		}
	}
}

func TestManifestBlobs_InvalidJSON(t *testing.T) {
	_, err := ManifestBlobs([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestWalker_ResolveImages_InvalidManifestBlob(t *testing.T) {
	tmpDir := t.TempDir()

	manifestDigest := "badmanifest"
	// Write invalid JSON as blob
	createMockDistributionLayout(t, tmpDir, "library/nginx", "latest", manifestDigest, []byte("not json"))

	w := NewWalker(tmpDir)
	_, err := w.ResolveImages([]string{"nginx:latest"})
	if err == nil {
		t.Fatal("expected error for invalid manifest blob")
	}
}

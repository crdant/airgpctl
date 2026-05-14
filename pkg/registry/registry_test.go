package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/replicatedhq/airgapctl/pkg/distribution"
)

// mockRegistry implements a minimal Docker Registry HTTP API V2 for testing.
type mockRegistry struct {
	server     *httptest.Server
	mu         sync.Mutex
	blobs      map[string]bool // key: "repo|digest"
	manifests  map[string][]byte // key: "repo|reference"
	tokenAuth  bool
	requests   []string // log of request paths for verification
}

func newMockRegistry(t *testing.T) *mockRegistry {
	mr := &mockRegistry{
		blobs:     make(map[string]bool),
		manifests: make(map[string][]byte),
		requests:  make([]string, 0),
	}

	mux := http.NewServeMux()

	// Blob checks
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		mr.mu.Lock()
		mr.requests = append(mr.requests, r.Method+" "+r.URL.Path)
		mr.mu.Unlock()

		if mr.tokenAuth && r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm=""`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// HEAD /v2/<name>/blobs/<digest>
		if r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/blobs/") {
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) >= 6 {
				digest := parts[len(parts)-1]
				repo := strings.Join(parts[2:len(parts)-2], "/")
				key := repo + "|" + digest
				mr.mu.Lock()
				exists := mr.blobs[key]
				mr.mu.Unlock()
				if exists {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			}
			return
		}

		// POST /v2/<name>/blobs/uploads/
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/blobs/uploads/") {
			w.Header().Set("Location", r.URL.Path+"upload-id")
			w.WriteHeader(http.StatusAccepted)
			return
		}

		// PUT /v2/<name>/blobs/uploads/upload-id?digest=<digest>
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/blobs/uploads/") {
			digest := r.URL.Query().Get("digest")
			parts := strings.Split(r.URL.Path, "/")
			repo := strings.Join(parts[2:len(parts)-3], "/")
			key := repo + "|" + digest
			// Read body to consume it
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
			mr.mu.Lock()
			mr.blobs[key] = true
			mr.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			return
		}

		// HEAD /v2/<name>/manifests/<reference>
		if r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/") {
			parts := strings.Split(r.URL.Path, "/")
			ref := parts[len(parts)-1]
			repo := strings.Join(parts[2:len(parts)-2], "/")
			key := repo + "|" + ref
			mr.mu.Lock()
			data, exists := mr.manifests[key]
			mr.mu.Unlock()
			if exists {
				w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}

		// PUT /v2/<name>/manifests/<reference>
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/") {
			parts := strings.Split(r.URL.Path, "/")
			ref := parts[len(parts)-1]
			repo := strings.Join(parts[2:len(parts)-2], "/")
			key := repo + "|" + ref
			data, _ := io.ReadAll(r.Body)
			r.Body.Close()
			mr.mu.Lock()
			mr.manifests[key] = data
			mr.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	mr.server = httptest.NewServer(mux)
	t.Cleanup(mr.server.Close)
	return mr
}

func TestNewPusher(t *testing.T) {
	mr := newMockRegistry(t)
	p := NewPusher(Config{
		Registry: mr.server.URL,
	})
	if p == nil {
		t.Fatal("expected pusher, got nil")
	}
}

func TestPusher_Push_SingleArch(t *testing.T) {
	mr := newMockRegistry(t)

	// Create a minimal bundle with one image
	tmpDir := t.TempDir()
	manifestDigest := "singlearch123"
	manifestJSON := []byte(`{
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
				"digest": "sha256:layerdigest"
			}
		]
	}`)
	createMockBlob(t, tmpDir, manifestDigest, manifestJSON)
	createMockBlob(t, tmpDir, "configdigest", []byte("config data"))
	createMockBlob(t, tmpDir, "layerdigest", []byte("layer data"))

	walker := distribution.NewWalker(tmpDir)
	img := distribution.Image{
		SourceRef:  "nginx:latest",
		Repository: "library/nginx",
		Tag:        "latest",
		Digest:     "sha256:" + manifestDigest,
		IsMultiArch: false,
	}

	p := NewPusher(Config{
		Registry: mr.server.URL,
	})

	reqs := []ImagePush{
		{Source: img, DestRepo: "prod/nginx", Tag: "latest"},
	}

	reports := p.Push(context.Background(), reqs, walker, nil)

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if !reports[0].Success {
		t.Fatalf("expected success, got error: %v", reports[0].Error)
	}
	if reports[0].Skipped {
		t.Error("expected not skipped for first push")
	}

	// Verify blobs were uploaded
	mr.mu.Lock()
	if !mr.blobs["prod/nginx|sha256:configdigest"] {
		t.Error("config blob not uploaded")
	}
	if !mr.blobs["prod/nginx|sha256:layerdigest"] {
		t.Error("layer blob not uploaded")
	}
	if mr.manifests["prod/nginx|latest"] == nil {
		t.Error("manifest not uploaded")
	}
	mr.mu.Unlock()
}

func TestPusher_Push_Idempotent(t *testing.T) {
	mr := newMockRegistry(t)

	// Pre-populate the registry with the manifest and blobs
	mr.mu.Lock()
	mr.blobs["prod/nginx|sha256:configdigest"] = true
	mr.blobs["prod/nginx|sha256:layerdigest"] = true
	mr.blobs["prod/nginx|sha256:manifest123"] = true
	mr.manifests["prod/nginx|latest"] = []byte("existing manifest")
	mr.mu.Unlock()

	tmpDir := t.TempDir()
	manifestJSON := []byte(`{
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
				"digest": "sha256:layerdigest"
			}
		]
	}`)
	createMockBlob(t, tmpDir, "manifest123", manifestJSON)
	createMockBlob(t, tmpDir, "configdigest", []byte("config data"))
	createMockBlob(t, tmpDir, "layerdigest", []byte("layer data"))

	walker := distribution.NewWalker(tmpDir)
	img := distribution.Image{
		SourceRef:   "nginx:latest",
		Repository:  "library/nginx",
		Tag:         "latest",
		Digest:      "sha256:manifest123",
		IsMultiArch: false,
	}

	p := NewPusher(Config{
		Registry: mr.server.URL,
	})

	reqs := []ImagePush{
		{Source: img, DestRepo: "prod/nginx", Tag: "latest"},
	}

	reports := p.Push(context.Background(), reqs, walker, nil)

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if !reports[0].Success {
		t.Fatalf("expected success, got error: %v", reports[0].Error)
	}
	if !reports[0].Skipped {
		t.Error("expected skipped for idempotent push")
	}
}

func TestPusher_Push_ProgressReporting(t *testing.T) {
	mr := newMockRegistry(t)

	tmpDir := t.TempDir()
	manifestDigest := "prog123"
	manifestJSON := []byte(`{
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
				"digest": "sha256:layerdigest"
			}
		]
	}`)
	createMockBlob(t, tmpDir, manifestDigest, manifestJSON)
	createMockBlob(t, tmpDir, "configdigest", []byte("config data"))
	createMockBlob(t, tmpDir, "layerdigest", []byte("layer data"))

	walker := distribution.NewWalker(tmpDir)
	img := distribution.Image{
		SourceRef:   "nginx:latest",
		Repository:  "library/nginx",
		Tag:         "latest",
		Digest:      "sha256:" + manifestDigest,
		IsMultiArch: false,
	}

	p := NewPusher(Config{
		Registry: mr.server.URL,
	})

	var progressEvents []Progress
	reqs := []ImagePush{
		{Source: img, DestRepo: "prod/nginx", Tag: "latest"},
	}

	reports := p.Push(context.Background(), reqs, walker, func(p Progress) {
		progressEvents = append(progressEvents, p)
	})

	if !reports[0].Success {
		t.Fatalf("expected success, got error: %v", reports[0].Error)
	}

	if len(progressEvents) == 0 {
		t.Fatal("expected progress events, got none")
	}

	// Should have start and completion events at minimum
	foundStart := false
	foundComplete := false
	for _, e := range progressEvents {
		if e.Current == 0 && e.Total == 1 {
			foundStart = true
		}
		if e.Current == 1 && e.Total == 1 && e.Image == "nginx:latest" {
			foundComplete = true
		}
	}
	if !foundStart {
		t.Error("expected start progress event")
	}
	if !foundComplete {
		t.Error("expected completion progress event with correct image name")
	}
}

func TestPusher_Push_MultiArch(t *testing.T) {
	mr := newMockRegistry(t)

	tmpDir := t.TempDir()

	// Create child manifests
	amd64Manifest := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": 7023,
			"digest": "sha256:amd64config"
		},
		"layers": [
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size": 32654,
				"digest": "sha256:amd64layer"
			}
		]
	}`)
	createMockBlob(t, tmpDir, "amd64manifest", amd64Manifest)
	createMockBlob(t, tmpDir, "amd64config", []byte("amd64 config"))
	createMockBlob(t, tmpDir, "amd64layer", []byte("amd64 layer"))

	arm64Manifest := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": 7023,
			"digest": "sha256:arm64config"
		},
		"layers": [
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size": 32654,
				"digest": "sha256:arm64layer"
			}
		]
	}`)
	createMockBlob(t, tmpDir, "arm64manifest", arm64Manifest)
	createMockBlob(t, tmpDir, "arm64config", []byte("arm64 config"))
	createMockBlob(t, tmpDir, "arm64layer", []byte("arm64 layer"))

	// Create manifest list
	listManifest := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests": [
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"size": 200,
				"digest": "sha256:amd64manifest",
				"platform": {
					"architecture": "amd64",
					"os": "linux"
				}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"size": 200,
				"digest": "sha256:arm64manifest",
				"platform": {
					"architecture": "arm64",
					"os": "linux"
				}
			}
		]
	}`)
	createMockBlob(t, tmpDir, "listdigest", listManifest)

	walker := distribution.NewWalker(tmpDir)
	img := distribution.Image{
		SourceRef:   "nginx:latest",
		Repository:  "library/nginx",
		Tag:         "latest",
		Digest:      "sha256:listdigest",
		IsMultiArch: true,
		Manifests: []distribution.PlatformManifest{
			{Digest: "sha256:amd64manifest", Platform: "linux/amd64"},
			{Digest: "sha256:arm64manifest", Platform: "linux/arm64"},
		},
	}

	p := NewPusher(Config{
		Registry: mr.server.URL,
	})

	reqs := []ImagePush{
		{Source: img, DestRepo: "prod/nginx", Tag: "latest"},
	}

	reports := p.Push(context.Background(), reqs, walker, nil)

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if !reports[0].Success {
		t.Fatalf("expected success, got error: %v", reports[0].Error)
	}

	// Verify all blobs and manifests uploaded
	mr.mu.Lock()
	blobChecks := []string{
		"prod/nginx|sha256:amd64config",
		"prod/nginx|sha256:amd64layer",
		"prod/nginx|sha256:arm64config",
		"prod/nginx|sha256:arm64layer",
	}
	for _, key := range blobChecks {
		if !mr.blobs[key] {
			t.Errorf("expected blob %s to be uploaded", key)
		}
	}
	manifestChecks := []string{
		"prod/nginx|sha256:amd64manifest",
		"prod/nginx|sha256:arm64manifest",
		"prod/nginx|latest",
	}
	for _, key := range manifestChecks {
		if mr.manifests[key] == nil {
			t.Errorf("expected manifest %s to be uploaded", key)
		}
	}
	mr.mu.Unlock()
}

func TestPusher_Push_Auth(t *testing.T) {
	mr := newMockRegistry(t)
	mr.tokenAuth = true

	tmpDir := t.TempDir()
	manifestDigest := "auth123"
	manifestJSON := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": 7023,
			"digest": "sha256:configdigest"
		},
		"layers": []
	}`)
	createMockBlob(t, tmpDir, manifestDigest, manifestJSON)
	createMockBlob(t, tmpDir, "configdigest", []byte("config data"))

	walker := distribution.NewWalker(tmpDir)
	img := distribution.Image{
		SourceRef:   "nginx:latest",
		Repository:  "library/nginx",
		Tag:         "latest",
		Digest:      "sha256:" + manifestDigest,
		IsMultiArch: false,
	}

	p := NewPusher(Config{
		Registry: mr.server.URL,
		Username: "admin",
		Password: "secret",
	})

	reqs := []ImagePush{
		{Source: img, DestRepo: "prod/nginx", Tag: "latest"},
	}

	reports := p.Push(context.Background(), reqs, walker, nil)

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if !reports[0].Success {
		t.Fatalf("expected success, got error: %v", reports[0].Error)
	}
}

func TestPusher_Push_PartialFailure(t *testing.T) {
	// Create a registry that rejects one specific blob
	mr := &mockRegistry{
		blobs:     make(map[string]bool),
		manifests: make(map[string][]byte),
		requests:  make([]string, 0),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		mr.mu.Lock()
		mr.requests = append(mr.requests, r.Method+" "+r.URL.Path)
		mr.mu.Unlock()

		if r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/blobs/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/blobs/uploads/") {
			w.Header().Set("Location", r.URL.Path+"upload-id")
			w.WriteHeader(http.StatusAccepted)
			return
		}

		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/blobs/uploads/") {
			digest := r.URL.Query().Get("digest")
			// Simulate failure for config digest
			if digest == "sha256:configdigest" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
			parts := strings.Split(r.URL.Path, "/")
			repo := strings.Join(parts[2:len(parts)-3], "/")
			key := repo + "|" + digest
			mr.mu.Lock()
			mr.blobs[key] = true
			mr.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	mr.server = httptest.NewServer(mux)
	defer mr.server.Close()

	tmpDir := t.TempDir()
	manifestDigest := "fail123"
	manifestJSON := []byte(`{
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
				"digest": "sha256:layerdigest"
			}
		]
	}`)
	createMockBlob(t, tmpDir, manifestDigest, manifestJSON)
	createMockBlob(t, tmpDir, "configdigest", []byte("config data"))
	createMockBlob(t, tmpDir, "layerdigest", []byte("layer data"))

	walker := distribution.NewWalker(tmpDir)
	img := distribution.Image{
		SourceRef:   "nginx:latest",
		Repository:  "library/nginx",
		Tag:         "latest",
		Digest:      "sha256:" + manifestDigest,
		IsMultiArch: false,
	}

	p := NewPusher(Config{
		Registry: mr.server.URL,
	})

	reqs := []ImagePush{
		{Source: img, DestRepo: "prod/nginx", Tag: "latest"},
	}

	reports := p.Push(context.Background(), reqs, walker, nil)

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Success {
		t.Fatal("expected failure due to blob upload error")
	}
	if reports[0].Error == nil {
		t.Fatal("expected error in report")
	}
}

// createMockBlob creates a blob in the tmpDir's blobs/sha256/<digest>/data path.
func createMockBlob(t *testing.T, dir, digest string, data []byte) {
	t.Helper()
	blobDir := dir + "/blobs/sha256/" + digest
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		t.Fatalf("creating blob dir: %v", err)
	}
	if err := os.WriteFile(blobDir+"/data", data, 0644); err != nil {
		t.Fatalf("writing blob data: %v", err)
	}
}

// demo/generate-bundle.go generates a realistic .airgap fixture bundle for use
// in the VHS demo and integration tests. It constructs a gzipped tar containing
// airgap.yaml and a Docker distribution v2 on-disk layout with one multi-arch
// image (nginx:latest) and two single-arch images (redis:7, postgres:15).
//
// Usage: go run demo/generate-bundle.go
package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The shared config/layer digest used by all single-arch manifests.
const sharedBlobDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Generated demo/fixtures/bundle.airgap")
}

func run() error {
	outPath := "demo/fixtures/bundle.airgap"
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("creating fixtures dir: %w", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating bundle file: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	w := tar.NewWriter(gw)
	defer w.Close()

	// 1. airgap.yaml
	yamlContent := `spec:
  savedImages:
  - "nginx:latest"
  - "redis:7"
  - "postgres:15"
`
	writeTarFile(w, "airgap.yaml", []byte(yamlContent))

	// 2. Build manifest payloads
	amd64JSON := makeSingleArchManifest()
	arm64JSON := makeSingleArchManifest()
	redisJSON := makeSingleArchManifest()
	postgresJSON := makeSingleArchManifest()

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
		return fmt.Errorf("marshaling manifest list: %w", err)
	}

	// 3. Write distribution layout for each image
	writeImageLayout(w, "library/nginx", "latest", "listdigest", listJSON)
	writeImageLayout(w, "library/redis", "7", "redis7digest", redisJSON)
	writeImageLayout(w, "library/postgres", "15", "postgres15digest", postgresJSON)

	// 4. Write child manifest blobs for the multi-arch image so pushMultiArch
	// can read them via walker.ReadBlob(pm.Digest).
	writeBlob(w, "amd64digest", amd64JSON)
	writeBlob(w, "arm64digest", arm64JSON)

	// 5. Write the shared config/layer blob referenced by all single-arch manifests
	writeBlob(w, sharedBlobDigest, []byte("dummy blob data"))

	return nil
}

func writeTarFile(w *tar.Writer, name string, data []byte) {
	header := &tar.Header{
		Name: name,
		Size: int64(len(data)),
		Mode: 0644,
	}
	if err := w.WriteHeader(header); err != nil {
		panic(fmt.Sprintf("writing tar header for %s: %v", name, err))
	}
	if _, err := w.Write(data); err != nil {
		panic(fmt.Sprintf("writing tar content for %s: %v", name, err))
	}
}

func writeImageLayout(w *tar.Writer, repo, tag, manifestDigest string, manifestJSON []byte) {
	// Manifest revision link
	revPath := fmt.Sprintf("images/docker/registry/v2/repositories/%s/_manifests/revisions/sha256/%s/link", repo, manifestDigest)
	writeTarFile(w, revPath, []byte("sha256:"+manifestDigest))

	// Tag current link
	tagPath := fmt.Sprintf("images/docker/registry/v2/repositories/%s/_manifests/tags/%s/current/link", repo, tag)
	writeTarFile(w, tagPath, []byte("sha256:"+manifestDigest))

	// Blob data with two-char prefix
	writeBlob(w, manifestDigest, manifestJSON)
}

func writeBlob(w *tar.Writer, digest string, data []byte) {
	prefix := ""
	if len(digest) >= 2 {
		prefix = digest[:2]
	}
	blobPath := fmt.Sprintf("images/docker/registry/v2/blobs/sha256/%s/%s/data", prefix, digest)
	writeTarFile(w, blobPath, data)
}

func makeSingleArchManifest() []byte {
	m := map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]interface{}{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size":      7023,
			"digest":    "sha256:" + sharedBlobDigest,
		},
		"layers": []interface{}{
			map[string]interface{}{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size":      32654,
				"digest":    "sha256:" + sharedBlobDigest,
			},
		},
	}
	data, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("marshaling manifest: %v", err))
	}
	return data
}

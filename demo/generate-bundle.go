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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
	if err := writeTarFile(w, "airgap.yaml", []byte(yamlContent)); err != nil {
		return err
	}

	// Create unique blobs for each image so digests differ
	amd64ConfigBlob := []byte(`{"architecture":"amd64","os":"linux"}`)
	arm64ConfigBlob := []byte(`{"architecture":"arm64","os":"linux"}`)
	redisConfigBlob := []byte(`{"architecture":"amd64","os":"linux","labels":{"app":"redis"}}`)
	postgresConfigBlob := []byte(`{"architecture":"amd64","os":"linux","labels":{"app":"postgres"}}`)

	// Create a shared layer blob with actual data
	layerBlob := []byte("dummy layer data for all images")

	// Compute digests for all blobs
	amd64ConfigDigest := sha256String(amd64ConfigBlob)
	arm64ConfigDigest := sha256String(arm64ConfigBlob)
	redisConfigDigest := sha256String(redisConfigBlob)
	postgresConfigDigest := sha256String(postgresConfigBlob)
	layerDigest := sha256String(layerBlob)

	// 2. Build manifest payloads
	amd64JSON := makeSingleArchManifest(amd64ConfigDigest, len(amd64ConfigBlob), layerDigest, len(layerBlob))
	arm64JSON := makeSingleArchManifest(arm64ConfigDigest, len(arm64ConfigBlob), layerDigest, len(layerBlob))
	redisJSON := makeSingleArchManifest(redisConfigDigest, len(redisConfigBlob), layerDigest, len(layerBlob))
	postgresJSON := makeSingleArchManifest(postgresConfigDigest, len(postgresConfigBlob), layerDigest, len(layerBlob))

	// Compute manifest digests
	amd64Digest := sha256String(amd64JSON)
	arm64Digest := sha256String(arm64JSON)
	redisDigest := sha256String(redisJSON)
	postgresDigest := sha256String(postgresJSON)

	listJSON, err := json.Marshal(map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests": []interface{}{
			map[string]interface{}{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"size":      len(amd64JSON),
				"digest":    "sha256:" + amd64Digest,
				"platform": map[string]string{
					"architecture": "amd64",
					"os":           "linux",
				},
			},
			map[string]interface{}{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"size":      len(arm64JSON),
				"digest":    "sha256:" + arm64Digest,
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
	listDigest := sha256String(listJSON)

	// 3. Write distribution layout for each image
	if err := writeImageLayout(w, "library/nginx", "latest", listDigest, listJSON); err != nil {
		return err
	}
	if err := writeImageLayout(w, "library/redis", "7", redisDigest, redisJSON); err != nil {
		return err
	}
	if err := writeImageLayout(w, "library/postgres", "15", postgresDigest, postgresJSON); err != nil {
		return err
	}

	// 4. Write child manifest revision links for the multi-arch image so pushMultiArch
	// can read them via walker.ReadBlob(pm.Digest).
	if err := writeManifestRevisionLink(w, "library/nginx", amd64Digest); err != nil {
		return err
	}
	if err := writeManifestRevisionLink(w, "library/nginx", arm64Digest); err != nil {
		return err
	}

	// 5. Write child manifest blobs for the multi-arch image
	if err := writeBlob(w, amd64Digest, amd64JSON); err != nil {
		return err
	}
	if err := writeBlob(w, arm64Digest, arm64JSON); err != nil {
		return err
	}

	// 6. Write config blobs
	if err := writeBlob(w, amd64ConfigDigest, amd64ConfigBlob); err != nil {
		return err
	}
	if err := writeBlob(w, arm64ConfigDigest, arm64ConfigBlob); err != nil {
		return err
	}
	if err := writeBlob(w, redisConfigDigest, redisConfigBlob); err != nil {
		return err
	}
	if err := writeBlob(w, postgresConfigDigest, postgresConfigBlob); err != nil {
		return err
	}

	// 7. Write the shared layer blob
	if err := writeBlob(w, layerDigest, layerBlob); err != nil {
		return err
	}

	return nil
}

func sha256String(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func writeTarFile(w *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name: name,
		Size: int64(len(data)),
		Mode: 0644,
	}
	if err := w.WriteHeader(header); err != nil {
		return fmt.Errorf("writing tar header for %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing tar content for %s: %w", name, err)
	}
	return nil
}

func writeImageLayout(w *tar.Writer, repo, tag, manifestDigest string, manifestJSON []byte) error {
	// Manifest revision link
	if err := writeManifestRevisionLink(w, repo, manifestDigest); err != nil {
		return err
	}

	// Tag current link
	tagPath := fmt.Sprintf("images/docker/registry/v2/repositories/%s/_manifests/tags/%s/current/link", repo, tag)
	if err := writeTarFile(w, tagPath, []byte("sha256:"+manifestDigest)); err != nil {
		return err
	}

	// Blob data with two-char prefix
	if err := writeBlob(w, manifestDigest, manifestJSON); err != nil {
		return err
	}
	return nil
}

func writeManifestRevisionLink(w *tar.Writer, repo, manifestDigest string) error {
	revPath := fmt.Sprintf("images/docker/registry/v2/repositories/%s/_manifests/revisions/sha256/%s/link", repo, manifestDigest)
	return writeTarFile(w, revPath, []byte("sha256:"+manifestDigest))
}

func writeBlob(w *tar.Writer, digest string, data []byte) error {
	prefix := ""
	if len(digest) >= 2 {
		prefix = digest[:2]
	}
	blobPath := fmt.Sprintf("images/docker/registry/v2/blobs/sha256/%s/%s/data", prefix, digest)
	return writeTarFile(w, blobPath, data)
}

func makeSingleArchManifest(configDigest string, configSize int, layerDigest string, layerSize int) []byte {
	m := map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]interface{}{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size":      configSize,
			"digest":    "sha256:" + configDigest,
		},
		"layers": []interface{}{
			map[string]interface{}{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size":      layerSize,
				"digest":    "sha256:" + layerDigest,
			},
		},
	}
	data, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("marshaling manifest: %v", err))
	}
	return data
}

// Package distribution provides functionality for traversing Docker distribution v2
// on-disk layouts, resolving manifest lists, and mapping SavedImages to on-disk paths.
package distribution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Image represents a resolved image reference within a Docker distribution v2 directory.
type Image struct {
	SourceRef   string             // Original image reference from airgap.yaml
	Repository  string             // Repository path on disk (e.g., "library/nginx")
	Tag         string             // Tag name (e.g., "latest")
	Digest      string             // Top-level manifest digest (sha256:...)
	IsMultiArch bool               // True if Digest is a manifest list / OCI index
	Manifests   []PlatformManifest // Platform-specific manifests (only for multi-arch)
}

// PlatformManifest represents a single platform-specific manifest within a manifest list.
type PlatformManifest struct {
	Digest    string // sha256:...
	Platform  string // e.g., "linux/amd64"
	MediaType string // e.g., "application/vnd.docker.distribution.manifest.v2+json"
}

// Walker traverses a Docker distribution v2 on-disk layout.
type Walker struct {
	basePath string
}

// NewWalker creates a new Walker for the given distribution directory.
// It auto-detects the nested Docker v2 layout (images/docker/registry/v2).
func NewWalker(basePath string) *Walker {
	// The Replicated .airgap bundle extracts to a directory where the Docker v2
	// registry layout is nested under images/docker/registry/v2.
	nested := filepath.Join(basePath, "images", "docker", "registry", "v2")
	if info, err := os.Stat(nested); err == nil && info.IsDir() {
		basePath = nested
	}
	return &Walker{basePath: basePath}
}

// ResolveImages maps SavedImages raw references to their on-disk Image metadata.
// It walks the images/ directory, matches repositories to the raw image names,
// reads the manifest blob, and detects multi-arch manifest lists.
func (w *Walker) ResolveImages(savedImages []string) ([]Image, error) {
	var result []Image

	for _, raw := range savedImages {
		img, err := w.resolveSingleImage(raw)
		if err != nil {
			return nil, fmt.Errorf("resolving %q: %w", raw, err)
		}
		result = append(result, *img)
	}

	return result, nil
}

// resolveSingleImage resolves one raw image reference to an Image.
func (w *Walker) resolveSingleImage(raw string) (*Image, error) {
	_, tag := parseImageRef(raw)

	// Map the raw image name to an on-disk repository path.
	repo, err := w.findRepo(raw)
	if err != nil {
		return nil, err
	}

	// Read the tag's current manifest digest
	tagLinkPath := filepath.Join(w.basePath, "repositories", repo, "_manifests", "tags", tag, "current", "link")
	linkData, err := os.ReadFile(tagLinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("tag %q not found for repository %q", tag, repo)
		}
		return nil, fmt.Errorf("reading tag link: %w", err)
	}

	digest := strings.TrimSpace(string(linkData))

	// Read the manifest blob
	manifestData, err := w.readBlob(digest)
	if err != nil {
		return nil, fmt.Errorf("reading manifest blob %s: %w", digest, err)
	}

	// Detect if it's a manifest list
	var manifestHeader struct {
		MediaType string `json:"mediaType"`
		Manifests []struct {
			MediaType string `json:"mediaType"`
			Size      int    `json:"size"`
			Digest    string `json:"digest"`
			Platform  struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(manifestData, &manifestHeader); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	img := &Image{
		SourceRef:  raw,
		Repository: repo,
		Tag:        tag,
		Digest:     digest,
	}

	// Determine if this is a manifest list / OCI index by mediaType or presence of manifests array
	if isManifestList(manifestHeader.MediaType) || len(manifestHeader.Manifests) > 0 {
		img.IsMultiArch = true
		for _, m := range manifestHeader.Manifests {
			platform := m.Platform.OS + "/" + m.Platform.Architecture
			img.Manifests = append(img.Manifests, PlatformManifest{
				Digest:    m.Digest,
				Platform:  platform,
				MediaType: m.MediaType,
			})
		}
	}

	return img, nil
}

// findRepo maps a raw saved image name to the on-disk repository directory.
// It first tries the full repository path, then falls back to matching by
// the last path component (image name) against the repositories/ directory.
func (w *Walker) findRepo(raw string) (string, error) {
	repo, _ := parseImageRef(raw)

	// Try the repo directly
	if _, err := os.Stat(filepath.Join(w.basePath, "repositories", repo)); err == nil {
		return repo, nil
	}

	// Fall back: list all on-disk repositories and find one whose last
	// path component matches the last component of the parsed repo.
	lastPart := filepath.Base(repo)

	entries, err := os.ReadDir(filepath.Join(w.basePath, "repositories"))
	if err != nil {
		return "", fmt.Errorf("listing repositories: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == lastPart {
			return entry.Name(), nil
		}
	}

	return "", fmt.Errorf("no on-disk repository found for %q", raw)
}

// readBlob reads a blob from the blobs/sha256/<xx>/<digest>/data path.
func (w *Walker) readBlob(digest string) ([]byte, error) {
	// digest is expected to be "sha256:<hex>"
	parts := strings.SplitN(digest, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid digest format: %s", digest)
	}
	algo, hash := parts[0], parts[1]
	if len(hash) < 2 {
		return nil, fmt.Errorf("invalid digest hash: %s", digest)
	}
	prefix := hash[:2]
	blobPath := filepath.Join(w.basePath, "blobs", algo, prefix, hash, "data")
	return os.ReadFile(blobPath)
}

// ReadBlob reads a content-addressable blob from the blobs/ directory.
func (w *Walker) ReadBlob(digest string) ([]byte, error) {
	return w.readBlob(digest)
}

// ManifestBlobs returns all blob digests referenced by a single-arch manifest.
// This includes the config blob and all layer blobs. It does NOT parse manifest lists.
func ManifestBlobs(manifestData []byte) ([]string, error) {
	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest for blob enumeration: %w", err)
	}

	var digests []string
	if manifest.Config.Digest != "" {
		digests = append(digests, manifest.Config.Digest)
	}
	for _, layer := range manifest.Layers {
		if layer.Digest != "" {
			digests = append(digests, layer.Digest)
		}
	}
	return digests, nil
}

// isManifestList returns true if the mediaType indicates a manifest list or OCI index.
func isManifestList(mediaType string) bool {
	switch mediaType {
	case "application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.index.v1+json":
		return true
	}
	return false
}

// parseImageRef splits a raw image reference like "nginx:latest" or
// "registry.replicated.com/myapp/myimage:v1.0.0" into (repository, tag).
func parseImageRef(raw string) (repo, tag string) {
	// Find the last colon that separates the tag from the name.
	// We must be careful about registry hosts with ports like "localhost:5000/repo:tag".
	// The tag colon is the last one after the last '/'.
	lastSlash := strings.LastIndex(raw, "/")
	namePart := raw
	if lastSlash != -1 {
		namePart = raw[lastSlash+1:]
	}

	colonIdx := strings.LastIndex(namePart, ":")
	if colonIdx == -1 {
		// No tag — default to "latest"
		repo = raw
		tag = "latest"
		return
	}

	// Adjust colonIdx to be relative to the full raw string
	if lastSlash != -1 {
		colonIdx += lastSlash + 1
	}

	repo = raw[:colonIdx]
	tag = raw[colonIdx+1:]

	// For Docker Hub official images without a registry prefix, add "library/"
	if !strings.Contains(repo, ".") && !strings.Contains(repo, ":") && !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}

	return repo, tag
}

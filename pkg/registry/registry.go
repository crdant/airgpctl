// Package registry provides functionality for pushing images to OCI-compatible registries
// directly from a Docker distribution v2 on-disk layout.
package registry

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/replicatedhq/airgapctl/pkg/distribution"
)

// Config holds connection and authentication parameters for the destination registry.
type Config struct {
	Registry string // Host (and optional port) of the destination registry
	Username string // Basic auth username (empty for anonymous)
	Password string // Basic auth password or token
	Token    string // Bearer token for registry authentication
	Insecure bool   // Skip TLS certificate verification
	// TODO: CustomCA []byte // PEM-encoded CA certificate for private registries (R2)
}

// ImagePush pairs a source image from the bundle with its destination repository and tag.
type ImagePush struct {
	Source   distribution.Image // Resolved source image metadata
	DestRepo string             // Destination repository path (e.g., "prod/nginx")
	Tag      string             // Destination tag (e.g., "latest")
}

// Progress is emitted during push to report which image is being processed.
type Progress struct {
	Current int    // Zero-based index of the current image
	Total   int    // Total number of images to push
	Image   string // Human-readable image reference (SourceRef)
}

// Report captures the outcome of pushing a single image.
type Report struct {
	Image   distribution.Image // The source image that was pushed
	Success bool               // True if the push completed without error
	Skipped bool               // True if the image was already present (idempotent skip)
	Error   error              // Non-nil if Success == false
}

// Pusher pushes images from a Docker distribution v2 on-disk layout to a remote registry.
type Pusher struct {
	cfg    Config
	client *http.Client
}

// defaultPushTimeout is the per-operation HTTP timeout for registry requests.
const defaultPushTimeout = 5 * time.Minute

// NewPusher creates a Pusher for the given registry configuration.
func NewPusher(cfg Config) *Pusher {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	// Normalize registry URL: ensure it has a scheme
	registryURL := cfg.Registry
	if !strings.HasPrefix(registryURL, "http://") && !strings.HasPrefix(registryURL, "https://") {
		registryURL = "https://" + registryURL
	}

	return &Pusher{
		cfg: Config{
			Registry: registryURL,
			Username: cfg.Username,
			Password: cfg.Password,
			Token:    cfg.Token,
			Insecure: cfg.Insecure,
		},
		client: &http.Client{
			Transport: transport,
			Timeout:   defaultPushTimeout,
		},
	}
}

// Push uploads the provided images to the destination registry.
// It reads blobs and manifests from the on-disk layout via the provided Walker.
// The onProgress callback is invoked before each image push and after each completion.
func (p *Pusher) Push(ctx context.Context, images []ImagePush, walker *distribution.Walker, onProgress func(Progress)) []Report {
	reports := make([]Report, 0, len(images))

	for i, req := range images {
		if err := ctx.Err(); err != nil {
			reports = append(reports, Report{Image: req.Source, Error: fmt.Errorf("context cancelled: %w", err)})
			continue
		}

		if onProgress != nil {
			onProgress(Progress{Current: i, Total: len(images), Image: req.Source.SourceRef})
		}

		report := p.pushOne(ctx, req, walker)
		reports = append(reports, report)

		if onProgress != nil {
			onProgress(Progress{Current: i + 1, Total: len(images), Image: req.Source.SourceRef})
		}
	}

	return reports
}

func (p *Pusher) pushOne(ctx context.Context, req ImagePush, walker *distribution.Walker) Report {
	repo := req.DestRepo
	ref := req.Tag

	// Idempotency check: if the manifest already exists, skip
	exists, err := p.manifestExists(ctx, repo, ref)
	if err != nil {
		return Report{Image: req.Source, Error: fmt.Errorf("checking manifest existence: %w", err)}
	}
	if exists {
		return Report{Image: req.Source, Success: true, Skipped: true}
	}

	if req.Source.IsMultiArch {
		return p.pushMultiArch(ctx, req, walker)
	}
	return p.pushSingleArch(ctx, req, walker)
}

func (p *Pusher) pushSingleArch(ctx context.Context, req ImagePush, walker *distribution.Walker) Report {
	repo := req.DestRepo

	// Read the manifest
	manifestData, err := walker.ReadBlob(req.Source.Digest)
	if err != nil {
		return Report{Image: req.Source, Error: fmt.Errorf("reading manifest: %w", err)}
	}

	// Enumerate and push all referenced blobs
	blobs, err := distribution.ManifestBlobs(manifestData)
	if err != nil {
		return Report{Image: req.Source, Error: fmt.Errorf("enumerating manifest blobs: %w", err)}
	}

	for _, digest := range blobs {
		if err := p.pushBlob(ctx, repo, digest, walker); err != nil {
			return Report{Image: req.Source, Error: fmt.Errorf("pushing blob %s: %w", digest, err)}
		}
	}

	// Push the manifest itself
	if err := p.pushManifest(ctx, repo, req.Tag, manifestData); err != nil {
		return Report{Image: req.Source, Error: fmt.Errorf("pushing manifest: %w", err)}
	}

	return Report{Image: req.Source, Success: true}
}

func (p *Pusher) pushMultiArch(ctx context.Context, req ImagePush, walker *distribution.Walker) Report {
	repo := req.DestRepo

	// Push each platform manifest first
	for _, pm := range req.Source.Manifests {
		manifestData, err := walker.ReadBlob(pm.Digest)
		if err != nil {
			return Report{Image: req.Source, Error: fmt.Errorf("reading platform manifest %s: %w", pm.Digest, err)}
		}

		blobs, err := distribution.ManifestBlobs(manifestData)
		if err != nil {
			return Report{Image: req.Source, Error: fmt.Errorf("enumerating platform manifest blobs: %w", err)}
		}

		for _, digest := range blobs {
			if err := p.pushBlob(ctx, repo, digest, walker); err != nil {
				return Report{Image: req.Source, Error: fmt.Errorf("pushing platform blob %s: %w", digest, err)}
			}
		}

		if err := p.pushManifest(ctx, repo, pm.Digest, manifestData); err != nil {
			return Report{Image: req.Source, Error: fmt.Errorf("pushing platform manifest %s: %w", pm.Digest, err)}
		}
	}

	// Push the manifest list
	listData, err := walker.ReadBlob(req.Source.Digest)
	if err != nil {
		return Report{Image: req.Source, Error: fmt.Errorf("reading manifest list: %w", err)}
	}

	if err := p.pushManifest(ctx, repo, req.Tag, listData); err != nil {
		return Report{Image: req.Source, Error: fmt.Errorf("pushing manifest list: %w", err)}
	}

	return Report{Image: req.Source, Success: true}
}

func (p *Pusher) manifestExists(ctx context.Context, repo, ref string) (bool, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", p.cfg.Registry, repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	p.setAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status %d from HEAD %s", resp.StatusCode, url)
	}
}

func (p *Pusher) pushBlob(ctx context.Context, repo, digest string, walker *distribution.Walker) error {
	// Check if blob already exists
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", p.cfg.Registry, repo, digest)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	p.setAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // Blob already exists
	}
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status %d checking blob %s", resp.StatusCode, digest)
	}

	// Read blob data from disk
	data, err := walker.ReadBlob(digest)
	if err != nil {
		return fmt.Errorf("reading blob %s from disk: %w", digest, err)
	}

	// Start upload
	uploadURL := fmt.Sprintf("%s/v2/%s/blobs/uploads/", p.cfg.Registry, repo)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, nil)
	if err != nil {
		return err
	}
	p.setAuth(req)

	resp, err = p.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d starting blob upload", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return fmt.Errorf("no Location header in upload response")
	}

	// Resolve relative Location header to absolute URL
	if strings.HasPrefix(location, "/") {
		location = p.cfg.Registry + location
	}

	// Complete upload with PUT
	putURL := location
	if !strings.Contains(location, "?") && !strings.Contains(location, "&") {
		putURL = location + "?" + "digest=" + digest
	} else if !strings.Contains(location, "digest=") {
		putURL = location + "&" + "digest=" + digest
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err = p.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status %d completing blob upload", resp.StatusCode)
	}

	return nil
}

func (p *Pusher) pushManifest(ctx context.Context, repo, ref string, data []byte) error {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", p.cfg.Registry, repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	p.setAuth(req)
	req.Header.Set("Content-Type", manifestMediaType(data))

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status %d pushing manifest", resp.StatusCode)
	}

	return nil
}

// manifestMediaType extracts the mediaType field from manifest JSON.
// It falls back to the Docker v2 manifest type if the field is absent.
func manifestMediaType(data []byte) string {
	var header struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(data, &header); err == nil && header.MediaType != "" {
		return header.MediaType
	}
	return "application/vnd.docker.distribution.manifest.v2+json"
}

func (p *Pusher) setAuth(req *http.Request) {
	if p.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.Token)
		return
	}
	if p.cfg.Username != "" || p.cfg.Password != "" {
		req.SetBasicAuth(p.cfg.Username, p.cfg.Password)
	}
}

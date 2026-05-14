---
title: "Pure Go registry push from Docker distribution v2 on-disk layout"
date: 2026-05-14
category: design-patterns
module: pkg/registry
problem_type: design_pattern
component: testing_framework
severity: medium
applies_when:
  - "Pushing container images from a Docker v2 on-disk bundle to a remote OCI registry"
  - "Needing idempotent push that skips already-present blobs and manifests"
  - "Handling multi-architecture manifest lists without external registry binaries"
tags:
  - oci-registry
  - docker-v2-layout
  - manifest-list
  - idempotent-push
  - mock-registry-testing
  - pure-go
---

# Pure Go registry push from Docker distribution v2 on-disk layout

## Context

In air-gapped environments, `.airgap` bundles contain container images in a Docker distribution v2 on-disk layout. Getting these images into a private registry typically requires `docker load` + `docker tag` + `docker push`, which assumes the `docker` binary is available. For a single static Go binary with zero runtime dependencies, we must push directly from the on-disk layout using the OCI Distribution Spec HTTP API.

## Guidance

### Mock the registry API for test-first development

Build a minimal `httptest.Server` that implements the subset of the Docker Registry HTTP API V2 needed for your push flow:

- `HEAD /v2/<name>/blobs/<digest>` → 200 (exists) or 404 (missing)
- `POST /v2/<name>/blobs/uploads/` → 202 with `Location` header
- `PUT <location>?digest=<digest>` → 201 (complete upload)
- `HEAD /v2/<name>/manifests/<reference>` → 200 (exists) or 404 (missing)
- `PUT /v2/<name>/manifests/<reference>` → 201 (upload manifest)

This lets you test single-arch push, multi-arch push, idempotency, auth, and partial failure without a real registry.

### Push order matters for multi-arch images

For a manifest list (multi-arch), the correct push order is:

1. For each platform manifest in the list:
   a. Push all referenced blobs (config + layers)
   b. Push the platform manifest by its digest
2. Push the manifest list itself by its tag

This ensures the list's digest references resolve correctly when the registry validates the upload.

### Idempotency via HEAD checks

Before uploading any blob or manifest, issue a `HEAD` request:

```go
func (p *Pusher) pushBlob(ctx context.Context, repo, digest string, walker *Walker) error {
    // Check existence first
    resp, err := p.headBlob(repo, digest)
    if resp.StatusCode == http.StatusOK {
        return nil // already present
    }
    // ... proceed with upload
}
```

For manifests, the idempotency check at the tag level prevents re-pushing an entire image when it has already been pushed. For blobs, per-blob skipping avoids redundant uploads of shared layers.

### Resolve relative Location headers

The registry's `POST /blobs/uploads/` response returns a `Location` header that may be absolute or relative. Always resolve relative paths against the registry base URL before following them:

```go
location := resp.Header.Get("Location")
if strings.HasPrefix(location, "/") {
    location = p.cfg.Registry + location
}
```

### Detect manifest media type from JSON

Registries validate the `Content-Type` header on manifest PUT requests. A manifest list with the wrong type will be rejected. Extract the `mediaType` field from the manifest JSON rather than hardcoding:

```go
func manifestMediaType(data []byte) string {
    var header struct {
        MediaType string `json:"mediaType"`
    }
    if err := json.Unmarshal(data, &header); err == nil && header.MediaType != "" {
        return header.MediaType
    }
    return "application/vnd.docker.distribution.manifest.v2+json"
}
```

### Context cancellation and timeouts

Respect `context.Context` at loop boundaries so a cancelled context stops the push between images rather than mid-blob. Set a per-HTTP-request timeout on the client to avoid hangs in air-gapped networks:

```go
client := &http.Client{
    Transport: transport,
    Timeout:   5 * time.Minute,
}
```

## Why This Matters

- **No runtime dependencies**: Pure Go HTTP eliminates the need for `docker`, `skopeo`, or `oras` binaries in the target environment
- **Fast idempotency**: HEAD checks are cheap; skipping existing content makes re-runs against the same registry near-instant
- **Testable without infrastructure**: The mock registry approach yields fast, deterministic tests that exercise the full push flow
- **Air-gap compatible**: All logic is self-contained; the binary only needs network reachability to the destination registry

## When to Apply

- Building a CLI that pushes images from a local bundle or cache directory
- Any tool that needs to operate in environments where container runtimes cannot be installed
- Scenarios where idempotent push is required (CI/CD, repeated sync operations)

## Examples

### Single-arch push

```go
p := registry.NewPusher(registry.Config{
    Registry: "https://myregistry.example.com",
    Username: "admin",
    Password: "secret",
})

reqs := []registry.ImagePush{
    {Source: img, DestRepo: "prod/nginx", Tag: "latest"},
}

reports := p.Push(ctx, reqs, walker, func(p registry.Progress) {
    fmt.Printf("[%d/%d] %s\n", p.Current, p.Total, p.Image)
})
```

### Multi-arch push

The same API handles manifest lists automatically. `IsMultiArch` on the source `distribution.Image` triggers the platform-first push order:

```go
for _, report := range reports {
    if report.Skipped {
        fmt.Printf("skipped (already present): %s\n", report.Image.SourceRef)
    } else if report.Success {
        fmt.Printf("pushed: %s\n", report.Image.SourceRef)
    } else {
        fmt.Printf("failed: %s — %v\n", report.Image.SourceRef, report.Error)
    }
}
```

### Partial failure handling

`Push` returns a `[]Report` with one entry per requested image. Failures are isolated: one image's blob upload error does not abort the remaining images. The caller decides whether to abort, retry, or report.

## Related

- `docs/solutions/design-patterns/template-based-helm-values-generation-airgap-images.md` — companion doc for generating values files that reference the pushed images
- `pkg/distribution/distribution.go` — on-disk layout traversal and `ManifestBlobs()` helper
- `pkg/registry/registry.go` — push implementation
- OCI Distribution Spec: https://github.com/opencontainers/distribution-spec/blob/main/spec.md

---
title: "Safe tar extraction and YAML parsing for .airgap bundles in Go"
date: 2026-05-14
category: design-patterns
module: bundle
problem_type: design_pattern
component: tooling
severity: medium
applies_when:
  - "Implementing archive extraction in Go CLI tools"
  - "Parsing structured metadata from bundled archives"
  - "Handling user-provided tar files that may contain malicious entries"
tags:
  - go
  - tar-extraction
  - yaml-parsing
  - directory-traversal
  - security
  - airgap-bundle
---

# Safe tar extraction and YAML parsing for .airgap bundles in Go

## Context

The `.airgap` bundle format used by Replicated is a tar archive containing Docker distribution v2 registry-on-disk layout plus an `airgap.yaml` metadata file. When building `airgapctl`, we needed to:

1. Open the `.airgap` file and locate `airgap.yaml` without fully extracting the archive
2. Parse `airgap.yaml` to discover the `SavedImages` field (the list of container images in the bundle)
3. Provide full extraction capability when needed, with protection against directory traversal attacks via malicious tar entries
4. Use pure Go with no external runtime dependencies (the binary must be self-sufficient in air-gapped environments)

## Guidance

### Two-phase approach

Separate **metadata discovery** from **full extraction**:

- **Phase 1 (OpenBundle)**: Stream the tar once to find `airgap.yaml`, parse it, and close the file. This is fast and memory-efficient because it does not extract anything to disk.
- **Phase 2 (ExtractTo)**: When the caller explicitly needs the full bundle contents on disk, stream the tar a second time and extract each entry safely.

### Directory traversal protection

When extracting tar entries, always validate that the target path stays within the destination directory:

```go
func isWithinDest(dest, target string) bool {
    rel, err := filepath.Rel(dest, target)
    if err != nil {
        return false
    }
    return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
```

This prevents symlink-based and `..`-based path escapes regardless of platform path separators.

### Prefer streaming over full extraction

Parse `airgap.yaml` directly from the tar stream using `io.ReadAll` on the tar entry. Do not extract the whole bundle to a temp directory just to read one small metadata file.

### Use standard library + minimal dependencies

- `archive/tar` from the Go standard library handles the bundle format.
- `gopkg.in/yaml.v3` is a lightweight, well-maintained YAML parser.
- No shelling out to `tar`, `docker`, or any other binary.

## Why This Matters

- **Security**: Malicious tar archives can overwrite arbitrary files via `..` path components (the "zip slip" vulnerability). Explicit traversal checking prevents this.
- **Performance**: Streaming metadata discovery avoids creating hundreds of megabytes of temporary files when only the image list is needed.
- **Air-gap compatibility**: Requiring only a static Go binary with zero runtime dependencies is a core product constraint.

## When to Apply

- Implementing any Go tool that reads tar-based bundle formats
- Extracting user-provided archives to the filesystem
- Parsing YAML metadata embedded inside tar archives

## Examples

### Opening a bundle (metadata only)

```go
b, err := bundle.OpenBundle("/path/to/bundle.airgap")
if err != nil {
    return err
}
fmt.Printf("Bundle version: %s, images: %d\n", b.Version, len(b.SavedImages))
```

### Extracting to a directory

```go
if err := b.ExtractTo("/tmp/extracted-bundle"); err != nil {
    return err
}
```

### Table-driven test for parser + security

```go
func TestOpenBundle(t *testing.T) {
    tests := []struct {
        name       string
        content    string
        wantImages []string
        wantErr    bool
    }{
        {
            name: "valid bundle",
            content: `Version: "1"
Type: "airgap"
SavedImages:
  - "nginx:latest"
`,
            wantImages: []string{"nginx:latest"},
            wantErr: false,
        },
        {
            name:       "missing airgap.yaml",
            content:    "",
            wantImages: nil,
            wantErr:    true,
        },
    }
    // ... table-driven execution
}

func TestBundle_ExtractTo_DirectoryTraversal(t *testing.T) {
    // Create a tar with "../../evil.txt" and assert extraction fails
    // and the evil file is NOT created outside the dest directory.
}
```

## Related

- `pkg/bundle/bundle.go` — implementation
- `pkg/bundle/bundle_test.go` — tests
- PRD requirements R4 (accept `.airgap` file) and R5 (parse `airgap.yaml`)

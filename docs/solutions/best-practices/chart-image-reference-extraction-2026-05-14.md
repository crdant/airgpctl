---
title: "Hardening chart image reference extraction in Go"
date: 2026-05-14
category: best-practices
module: chart-image-extraction
problem_type: best_practice
component: tooling
severity: medium
last_updated: 2026-05-14
applies_when:
  - "Parsing Helm chart values.yaml to discover image references"
  - "Extracting Helm chart tarballs to inspect contents"
  - "Matching user-provided references against a known image list"
  - "Implementing robust YAML string extraction without a full parser"
tags:
  - helm-charts
  - image-extraction
  - yaml-parsing
  - string-matching
  - tar-extraction
  - go
---

# Hardening chart image reference extraction in Go

## Context

When generating a Helm values file that remaps container images to a private registry, `airgapctl` must discover which images a chart actually references. This involves three operations that each have subtle edge cases:

1. **Parsing `values.yaml` as text** to find `repository:`, `image:`, and array-item image references, because the chart values are user-authored and may use inconsistent quoting.
2. **Detecting chart tarballs** so local `.tgz` / `.tar.gz` files can be extracted before reading `values.yaml`.
3. **Matching extracted references against the bundle image list** so only relevant images are remapped.

An initial implementation used loose heuristics that produced false positives, missed legitimate references, and accepted unsafe archive formats.

## Guidance

### Use prefix checks, not substring checks, for YAML key extraction

When scanning YAML line-by-line, `strings.Index` or `strings.Contains` will match keys that merely *contain* the target string (e.g. `some_repository:` matching `repository:`). Always use `strings.HasPrefix` after trimming whitespace:

```go
line = strings.TrimSpace(line)
if strings.HasPrefix(line, "repository:") {
    repo := strings.TrimSpace(line[len("repository:"):])
    repo = strings.Trim(repo, `"'`)
    // ...
}
if strings.HasPrefix(line, "image:") {
    img := strings.TrimSpace(line[len("image:"):])
    img = strings.Trim(img, `"'`)
    // ...
}
```

### Strip both single and double quotes from YAML scalar values

Helm chart authors quote scalars inconsistently. A value parser that only strips `"` will leave single-quoted values like `'nginx:alpine'` with literal quote characters, producing invalid image references. Use a combined cut-set:

```go
value = strings.Trim(value, `"'`)
```

### Relax image-reference heuristics for Docker Hub short names

A heuristic that requires `strings.Contains(s, "/")` rejects valid Docker Hub references such as `nginx:alpine`. When matching array items that might be image references, test for the presence of a tag separator (`:`) instead of requiring a registry/path separator (`/`):

```go
func looksLikeImageRef(s string) bool {
    return strings.Contains(s, ":")
}
```

### Match chart references by exact short-name equality

Bidirectional `strings.Contains` between a chart reference and a bundle image name causes false positives: `app` matches `myapp`, and `redis` matches `predis`. Resolve both sides to their short name (last path component) and compare with exact equality:

```go
func (c *ChartMatcher) Match(name string) bool {
    shortName := imageName(name)
    for _, ref := range c.refs {
        if shortName == imageName(ref) {
            return true
        }
    }
    return false
}
```

### Validate chart-tarball extensions precisely

Accepting any `.gz` file as a chart tarball will misidentify non-chart archives. Restrict detection to the exact Helm chart extensions `.tgz` and `.tar.gz`:

```go
func isChartTarball(path string) bool {
    ext := strings.ToLower(filepath.Ext(path))
    return ext == ".tgz" || strings.HasSuffix(path, ".tar.gz")
}
```

### Skip PAX extended headers when iterating tar archives

The Go `archive/tar` reader emits `tar.TypeXHeader` and `tar.TypeXGlobalHeader` entries for PAX extended attributes. Treating them as regular files causes extraction errors or spurious empty files. Add explicit skip cases in the extraction loop:

```go
switch header.Typeflag {
case tar.TypeDir:
    // ...
case tar.TypeReg:
    // ...
case tar.TypeSymlink, tar.TypeLink:
    continue // skip symlinks/hard links for security
case tar.TypeXHeader, tar.TypeXGlobalHeader:
    continue // skip PAX extended headers
}
```

### Remove dead code immediately

Unused struct fields, abandoned variables, and commented-out code that survived a refactor should be deleted in the same commit that obsoletes them. They create confusion during later reviews and waste maintenance time.

## Why This Matters

- **False positives waste compute and storage**: Remapping images a chart does not actually use pollutes the generated values file and may push unnecessary layers to the air-gapped registry.
- **False negatives break deployments**: Missing a legitimate image reference (because quotes were not stripped or a short name lacked `/`) leaves the chart pointing to an unreachable upstream registry.
- **Imprecise archive handling is a security risk**: Extracting non-chart archives or mishandling tar headers can lead to traversal attacks or corrupted working directories.
- **Dead code compounds technical debt**: Every unused field is a potential target of future "fixes" that do nothing, wasting review cycles.

## When to Apply

- Any tool that parses Helm `values.yaml` or similar lightly-structured YAML without a schema-aware parser
- Image-discovery logic that must tolerate Docker Hub short names (`nginx:alpine`) alongside fully-qualified references (`registry.io/ns/app:1.0`)
- String-matching logic where user input is matched against a controlled vocabulary (use exact or prefix matching, never bidirectional `Contains`)
- Tar extraction code that may encounter modern GNU or PAX-format archives

## Examples

### Robust `extractImageRefs` implementation

```go
func extractImageRefs(data string) []string {
    var refs []string
    for _, line := range strings.Split(data, "\n") {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "repository:") {
            repo := strings.TrimSpace(line[len("repository:"):])
            repo = strings.Trim(repo, `"'`)
            if repo != "" {
                refs = append(refs, repo)
            }
        }
        if strings.HasPrefix(line, "image:") {
            img := strings.TrimSpace(line[len("image:"):])
            img = strings.Trim(img, `"'`)
            if img != "" {
                refs = append(refs, img)
            }
        }
        if strings.HasPrefix(line, "- ") {
            item := strings.TrimPrefix(line, "- ")
            item = strings.TrimSpace(item)
            item = strings.Trim(item, `"'`)
            if item != "" && strings.Contains(item, ":") {
                refs = append(refs, item)
            }
        }
    }
    return refs
}
```

### Exact-match chart filtering tests

```go
func TestChartMatcher(t *testing.T) {
    matcher := values.NewChartMatcher([]string{"nginx", "redis"})

    tests := []struct {
        name string
        want bool
    }{
        {"nginx", true},
        {"library/nginx", true},          // short name matches chart ref "nginx"
        {"docker.io/library/nginx", true}, // short name matches chart ref "nginx"
        {"nginx-plus", false},            // substring false positive eliminated
        {"my-nginx", false},              // substring false positive eliminated
        {"predis", false},                // substring false positive eliminated
        {"postgres", false},
    }
    // ...
}
```

## Related

- `docs/solutions/design-patterns/safe-tar-extraction-yaml-parsing-airgap-bundles.md` — overlapping area (tar/YAML handling) focused on bundle format security rather than chart reference extraction
- `cmd/airgapctl/values.go` — implementation of `extractImageRefs`, `isChartTarball`, `extractTarball`
- `pkg/values/values.go` — `ChartMatcher`, `RemapChartValues`, and chart-level matching logic
- `pkg/distribution/reference.go` — shared `ImageName`, `ImagePath`, and `StripTag` helpers (moved here during U3)
- Implementation commit: `1ab3483`
- Review-fixes commit: `50f93d2`

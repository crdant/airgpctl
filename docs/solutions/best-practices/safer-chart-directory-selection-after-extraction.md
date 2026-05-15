---
title: Safer chart directory selection after extraction
date: 2026-05-14
category: best-practices
module: chart-image-extraction
problem_type: best_practice
component: tooling
severity: medium
applies_when:
  - "Locating a Helm chart directory after helm pull --untar or manual tarball extraction"
  - "Matching an extracted chart against its expected name from an OCI reference or tarball filename"
  - "Guarding against empty paths, missing Chart.yaml, and empty extraction roots"
tags:
  - helm-charts
  - chart-discovery
  - filepath-guards
  - go
  - edge-cases
---

# Safer chart directory selection after extraction

## Context

After extracting a Helm chart—whether via `helm pull --untar` or by manually unpacking a `.tgz`/`.tar.gz` archive—the resulting directory layout places the chart contents inside a single subdirectory. The caller needs to locate that subdirectory reliably. Relying on the directory name alone is fragile because:

- `helm --untar` names the subdirectory after the chart, but the exact name may differ from the OCI reference or tarball base name.
- The user may pass an OCI reference (e.g., `oci://registry/chart`) or a local tarball path, so the expected chart name must be derived differently in each case.
- Extraction may produce an empty root, or a subdirectory that lacks `Chart.yaml`, both of which must be handled gracefully.

## Guidance

### Derive the expected chart name from the source reference, not the filesystem

When the chart comes from an **OCI reference**, use the last path component:

```go
expectedName := filepath.Base(valuesOpts.chart) // "mychart" from "oci://registry.io/ns/mychart"
```

When the chart comes from a **tarball**, strip the `.tgz` or `.tar.gz` extension from the filename. Do **not** assume the base name is already a valid chart name—guard against an empty input path:

```go
func chartNameFromTarball(path string) string {
    if path == "" {
        return ""
    }
    base := filepath.Base(path)
    lower := strings.ToLower(base)
    if strings.HasSuffix(lower, ".tar.gz") {
        return base[:len(base)-7]
    }
    if strings.HasSuffix(lower, ".tgz") {
        return base[:len(base)-4]
    }
    return base
}
```

**Why the empty guard matters**: `filepath.Base("")` returns `"."`, which would trick downstream matching logic into looking for a chart literally named `.`.

### Match by parsing `Chart.yaml` `metadata.name`, not by directory name

After extraction, iterate the immediate subdirectories of the root and read each candidate's `Chart.yaml`. Compare `metadata.name` against the expected chart name. Directory names can differ from chart metadata (e.g., versioned directory names, temp prefixes), so the YAML field is the canonical identifier:

```go
func findChartDir(root string, expectedName string) (string, error) {
    entries, err := os.ReadDir(root)
    if err != nil {
        return "", fmt.Errorf("reading extracted chart directory: %w", err)
    }

    var firstDir string
    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }
        dirPath := filepath.Join(root, entry.Name())
        if firstDir == "" {
            firstDir = dirPath
        }

        chartYAMLPath := filepath.Join(dirPath, "Chart.yaml")
        data, err := os.ReadFile(chartYAMLPath)
        if err != nil {
            continue
        }

        var meta struct {
            Name string `yaml:"name"`
        }
        if err := yaml.Unmarshal(data, &meta); err != nil {
            continue
        }
        if meta.Name == expectedName {
            return dirPath, nil
        }
    }

    if firstDir == "" {
        return "", fmt.Errorf("no chart directory found in %s", root)
    }
    return firstDir, nil
}
```

### Fallback to the first subdirectory when `Chart.yaml` is missing

If no subdirectory contains a parseable `Chart.yaml` with a matching name, fall back to the first subdirectory rather than failing outright. This preserves backward compatibility with loosely packaged charts while still surfacing a clear error when the root is completely empty.

### Return an explicit error when the root has no subdirectories

An empty extraction root (no subdirectories at all) is a definite failure condition—return an error immediately instead of silently falling back to an empty string or the root itself.

## Why This Matters

- **Directory names are not API contracts**: `helm --untar` and manual tar extraction may name directories after the chart, but they may also include version strings, temp prefixes, or platform-specific variations. Only `Chart.yaml` `name` is authoritative.
- **Empty inputs propagate silently**: Without the empty-string guard, `filepath.Base("")` yields `"."`, which later propagates into confusing match failures or false positives.
- **Graceful fallback reduces brittleness**: Charts repackaged by third-party tools sometimes omit `Chart.yaml` or place it unexpectedly. Falling back to the first subdirectory lets the tool continue when the metadata is missing, while still failing loudly when nothing is present at all.

## When to Apply

- Any workflow that extracts a Helm chart and then needs to locate the chart root for further inspection
- Tools that accept both OCI references and local tarball paths and must normalize them to a chart name
- Go utilities using `filepath.Base` where the input may be user-provided and therefore empty

## Examples

### Unit-test coverage for the tarball name parser

```go
func TestChartNameFromTarball(t *testing.T) {
    tests := []struct {
        path string
        want string
    }{
        {"chart.tgz", "chart"},
        {"chart.tar.gz", "chart"},
        {"chart.TGZ", "chart"},
        {"chart.TAR.GZ", "chart"},
        {"/path/to/chart.tgz", "chart"},
        {"/path/to/my-chart-1.2.3.tar.gz", "my-chart-1.2.3"},
        {"chart.yaml", "chart.yaml"},
        {"chart", "chart"},
        {"", ""},
    }
    for _, tt := range tests {
        t.Run(tt.path, func(t *testing.T) {
            got := chartNameFromTarball(tt.path)
            if got != tt.want {
                t.Errorf("chartNameFromTarball(%q) = %q, want %q", tt.path, got, tt.want)
            }
        })
    }
}
```

### Unit-test coverage for empty root and missing Chart.yaml

```go
func TestFindChartDir_NoDirs(t *testing.T) {
    tmpDir := t.TempDir()
    _, err := findChartDir(tmpDir, "missing")
    if err == nil {
        t.Fatal("expected error when no chart directories exist, got nil")
    }
}

func TestFindChartDir_MissingChartYAML(t *testing.T) {
    tmpDir := t.TempDir()
    chartDir := filepath.Join(tmpDir, "chart")
    if err := os.MkdirAll(chartDir, 0755); err != nil {
        t.Fatalf("creating chart dir: %v", err)
    }

    got, err := findChartDir(tmpDir, "chart")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    want := chartDir
    if got != want {
        t.Errorf("findChartDir(%q, %q) = %q, want %q", tmpDir, "chart", got, want)
    }
}
```

## Related

- `docs/solutions/best-practices/chart-image-reference-extraction-2026-05-14.md` — sibling guidance on extracting image references once the chart directory is located
- `cmd/airgapctl/values.go` — implementation of `findChartDir`, `chartNameFromTarball`, and tarball/untar flows
- Implementation and review-fixes commit: `73311cc`

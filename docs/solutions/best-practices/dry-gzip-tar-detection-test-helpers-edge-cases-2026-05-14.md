---
title: "DRY gzip/tar detection, deduplicated test helpers, and edge-case coverage in Go"
date: 2026-05-14
category: best-practices
module: bundle
problem_type: best_practice
component: tooling
severity: medium
applies_when:
  - "Writing Go code that probes file formats before parsing"
  - "Maintaining test helpers that are nearly identical"
  - "Reviewing code that handles edge cases without explicit test coverage"
  - "Handling third-party library errors where some codes mean 'not applicable' and others mean 'broken'"
tags:
  - go
  - gzip
  - tar
  - dry-principle
  - test-helpers
  - edge-cases
  - error-handling
  - code-review
---

# DRY gzip/tar detection, deduplicated test helpers, and edge-case coverage in Go

## Context

During implementation of U4 (graceful gzip vs. plain-tar detection for `.airgap` bundles), the initial commit (`d241cb9`) introduced the same gzip-probe-and-seekback logic in both `OpenBundle` and `ExtractTo`. A follow-up code review identified three categories of issues that are generic to Go development, not specific to bundle handling:

1. **DRY violation**: Two public functions duplicated the same file-open → gzip-probe → seek-back → tar-reader sequence.
2. **Test-helper duplication**: `createMockAirgapBundle` (gzipped) and `createPlainTarBundle` (plain tar) were ~90% identical.
3. **Missing edge-case test**: A 0-byte file is an interesting boundary condition that should be documented with a test even if the current code handles it correctly.
4. **Imprecise error handling**: The initial implementation treated *any* `gzip.NewReader` error as "not gzip," but `gzip.ErrHeader` is the only signal that means "this file is plain data." An EOF or corrupted header is a real error.

## Guidance

### 1. Extract shared format-detection helpers

When two or more functions need the same "probe file format, then set up the right reader" sequence, extract a helper that returns the primitive components so callers retain control over lifecycle (closing, deferring):

```go
// openBundleFile opens the bundle at path, detects whether it is gzip-compressed,
// and returns the underlying file, an optional gzip.Reader, and a tar.Reader.
// The caller is responsible for closing the file and the gzip.Reader.
func openBundleFile(path string) (*os.File, *gzip.Reader, *tar.Reader, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, nil, nil, fmt.Errorf("opening bundle: %w", err)
    }

    gr, err := gzip.NewReader(f)
    if err == gzip.ErrHeader {
        if _, err := f.Seek(0, io.SeekStart); err != nil {
            f.Close()
            return nil, nil, nil, fmt.Errorf("seeking bundle file: %w", err)
        }
        return f, nil, tar.NewReader(f), nil
    } else if err != nil {
        f.Close()
        return nil, nil, nil, fmt.Errorf("decompressing bundle: %w", err)
    }

    return f, gr, tar.NewReader(gr), nil
}
```

Why return the raw `*os.File` and `*gzip.Reader` instead of an opaque wrapper? Because `OpenBundle` and `ExtractTo` need to `defer` close them in different orders and under different conditions. Returning primitives keeps the helper pure and the callers explicit.

### 2. Merge nearly identical test helpers with a parameter

When two helpers differ only by a single predicate (e.g., "use gzip" vs. "don't use gzip"), merge them into one helper with a flag or options struct:

**Before (duplicated)**

```go
func createMockAirgapBundle(t *testing.T, dir string, content string) string {
    // ... opens file, creates gzip.NewWriter, tar.NewWriter(gw) ...
}

func createPlainTarBundle(t *testing.T, dir string, content string) string {
    // ... opens file, creates tar.NewWriter(f) ...
}
```

**After (parameterized)**

```go
func createMockBundle(t *testing.T, dir string, content string, gzipped bool) string {
    t.Helper()
    bundlePath := filepath.Join(dir, "test.airgap")
    f, err := os.Create(bundlePath)
    if err != nil {
        t.Fatalf("creating mock bundle: %v", err)
    }
    defer f.Close()

    var w *tar.Writer
    if gzipped {
        gw := gzip.NewWriter(f)
        defer gw.Close()
        w = tar.NewWriter(gw)
    } else {
        w = tar.NewWriter(f)
    }
    defer w.Close()

    header := &tar.Header{
        Name: "airgap.yaml",
        Size: int64(len(content)),
        Mode: 0644,
    }
    if err := w.WriteHeader(header); err != nil {
        t.Fatalf("writing tar header: %v", err)
    }
    if _, err := w.Write([]byte(content)); err != nil {
        t.Fatalf("writing tar content: %v", err)
    }

    return bundlePath
}
```

This eliminates drift: when the tar header format changes (e.g., adding PAX headers), there is only one site to update.

### 3. Add edge-case tests even when the code "already handles it"

A 0-byte file is a natural boundary condition. If the current code returns an error (because `gzip.NewReader` on an empty file returns an error other than `ErrHeader`), a test documents that behavior and prevents a future refactor from accidentally "fixing" it into a silent success:

```go
func TestOpenBundle_EmptyFile(t *testing.T) {
    tmpDir := t.TempDir()
    bundlePath := filepath.Join(tmpDir, "empty.airgap")
    if err := os.WriteFile(bundlePath, []byte{}, 0644); err != nil {
        t.Fatalf("creating empty file: %v", err)
    }

    _, err := OpenBundle(bundlePath)
    if err == nil {
        t.Fatal("expected error for empty file, got nil")
    }
}
```

The test cost is near zero; the documentation value is high.

### 4. Discriminate third-party "not applicable" errors from real errors

`gzip.NewReader` can fail for many reasons. Only `gzip.ErrHeader` means "this stream is not gzip-compressed." Other errors (EOF, unexpected EOF, corrupted data) are genuine failures and must propagate:

```go
gr, err := gzip.NewReader(f)
if err == gzip.ErrHeader {
    // Plain tar — seek back and proceed
    if _, err := f.Seek(0, io.SeekStart); err != nil {
        return nil, nil, nil, fmt.Errorf("seeking bundle file: %w", err)
    }
    return f, nil, tar.NewReader(f), nil
} else if err != nil {
    // Real decompression error — propagate
    f.Close()
    return nil, nil, nil, fmt.Errorf("decompressing bundle: %w", err)
}
```

## Why This Matters

- **DRY prevents silent drift**: If gzip detection rules change (e.g., supporting bzip2 or zstd), a single helper guarantees both `OpenBundle` and `ExtractTo` stay consistent.
- **Test-helper deduplication reduces maintenance**: Two helpers that do the same thing invite "I updated one but forgot the other" bugs during refactors.
- **Edge-case tests are executable documentation**: They prevent well-meaning refactors from accidentally changing behavior on boundary conditions.
- **Imprecise error swallowing masks real bugs**: Treating all `gzip.NewReader` errors as "not gzip" would hide corrupted bundles, truncated files, or I/O failures, leading to confusing downstream errors.

## When to Apply

- Any time two or more functions perform the same file-format probe and reader setup
- When test helpers differ by a single boolean or enum flag
- When a boundary condition (empty file, max-size file, nil input) is reachable but untested
- When a library uses a sentinel error to mean "this algorithm doesn't apply here" alongside other errors that mean "this algorithm failed"

## Examples

### Before-and-after for DRY format detection

**Before (duplicated in two functions)**

```go
func OpenBundle(path string) (*Bundle, error) {
    f, err := os.Open(path)
    // ... 8 lines of gzip probe + tar.NewReader ...
}

func (b *Bundle) ExtractTo(dest string) error {
    f, err := os.Open(b.bundlePath)
    // ... identical 8 lines ...
}
```

**After (shared helper)**

```go
func openBundleFile(path string) (*os.File, *gzip.Reader, *tar.Reader, error) { /* ... */ }

func OpenBundle(path string) (*Bundle, error) {
    f, gr, tr, err := openBundleFile(path)
    // ... caller handles defer/close ...
}

func (b *Bundle) ExtractTo(dest string) error {
    f, gr, tr, err := openBundleFile(b.bundlePath)
    // ... caller handles defer/close ...
}
```

### Error discrimination with gzip

| `gzip.NewReader` error | Meaning | Action |
|---|---|---|
| `nil` | Valid gzip stream | Wrap with `tar.NewReader(gr)` |
| `gzip.ErrHeader` | Not a gzip file | Seek to start, wrap with `tar.NewReader(f)` |
| `io.EOF` | Empty file | Propagate as error |
| `io.ErrUnexpectedEOF` | Truncated/corrupted | Propagate as error |
| Any other error | I/O or corruption | Propagate as error |

## Related

- `pkg/bundle/bundle.go` — implementation of `openBundleFile`, `OpenBundle`, and `ExtractTo`
- `pkg/bundle/bundle_test.go` — tests including `TestOpenBundle_EmptyFile`, `TestOpenBundle_PlainTar`, `TestOpenBundle_GzippedTar`
- Commit `d241cb9` — U4 initial implementation
- Commit `cad17bf` — review-fix refactor extracting `openBundleFile` and deduplicating `createMockBundle`
- `docs/solutions/design-patterns/safe-tar-extraction-yaml-parsing-airgap-bundles.md` — sibling doc covering tar extraction security and streaming patterns

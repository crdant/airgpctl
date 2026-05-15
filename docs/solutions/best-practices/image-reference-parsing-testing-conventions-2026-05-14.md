---
title: "Parsing Docker image references and testing string utilities in Go"
date: 2026-05-14
category: best-practices
module: image-reference-parsing
problem_type: best_practice
component: tooling
severity: medium
applies_when:
  - "Extracting short image names or repository paths from full Docker image references"
  - "Normalizing references that may contain tags, digests, or registry ports"
  - "Writing table-driven tests for string-parsing utilities in Go"
tags:
  - docker-images
  - image-references
  - go
  - parsing
  - testing
  - table-driven-tests
---

# Parsing Docker image references and testing string utilities in Go

## Context

During U3, image-path helpers (`ImageName`, `ImagePath`, `StripTag`) were deduplicated from `cmd/airgapctl/push.go` and `pkg/values/values.go` into a shared `pkg/distribution/reference.go`. Code review surfaced two issues:

1. **`ImageName` preserved digest references** (`@sha256:...`) instead of extracting the short name. A reference like `docker.io/library/nginx@sha256:abc123` produced `nginx@sha256:abc123` instead of `nginx`, which broke chart image matching and filtering for digest-based bundle images.
2. **Table-driven tests used anonymous subtests**, making failures hard to diagnose. When a case failed, the output only showed the input and expected values, not a human-readable case name.

## Guidance

### Strip digest before tag when normalizing image references

Docker image references may contain a tag (`:`), a digest (`@`), or both. When extracting the short name or repository path, always remove the digest first, then remove the tag. Reversing the order causes a digest reference to survive normalization:

```go
// ImageName extracts the short image name from a repository path or full reference.
func ImageName(repo string) string {
	// Strip digest if present
	if idx := strings.LastIndex(repo, "@"); idx != -1 {
		repo = repo[:idx]
	}
	// Strip tag if present
	if idx := strings.LastIndex(repo, ":"); idx > strings.LastIndex(repo, "/") {
		repo = repo[:idx]
	}
	idx := strings.LastIndex(repo, "/")
	if idx == -1 {
		return repo
	}
	return repo[idx+1:]
}
```

The tag-stripping condition `idx > strings.LastIndex(repo, "/")` is critical: without it, a registry port like `localhost:5000` is mistaken for a tag separator and incorrectly stripped.

### Cover edge cases in parser tests

Image references are surprisingly varied. A minimal test suite should cover at least:

- Library path: `library/nginx` → `nginx`
- Full reference with tag: `registry.com/ns/app:1.0` → `app`
- Plain short name: `nginx` → `nginx`
- Empty string: `""` → `""`
- Digest-only reference: `docker.io/library/nginx@sha256:abc123` → `nginx`
- Combined tag and digest: `registry.com/ns/app:1.0@sha256:abc123` → `app`
- Port registry with digest: `localhost:5000/nginx@sha256:abc123` → `nginx`

### Use `t.Run` with named subtests for all table-driven tests

Anonymous subtests force developers to mentally map line numbers to test data. Name each case so `go test -v` and failure output read like a checklist:

```go
func TestImageName(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"library path", "library/nginx", "nginx"},
		{"full ref with tag", "registry.com/ns/app:1.0", "app"},
		{"plain name", "nginx", "nginx"},
		{"empty", "", ""},
		{"digest ref", "docker.io/library/nginx@sha256:abc123", "nginx"},
		{"tag and digest", "registry.com/ns/app:1.0@sha256:abc123", "app"},
		{"port registry digest", "localhost:5000/nginx@sha256:abc123", "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ImageName(tt.ref)
			if got != tt.want {
				t.Errorf("ImageName(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}
```

## Why This Matters

- **Incorrect normalization breaks downstream matching.** If `ImageName("nginx@sha256:abc123")` returns `nginx@sha256:abc123`, a chart matcher looking for the short name `nginx` will fail to find it, causing false negatives in the generated values file.
- **Port-registry references are common in local and air-gapped testing.** A parser that strips `localhost:5000` to `localhost` because it treats the port as a tag separator produces invalid registry paths.
- **Named subtests turn triage into reading.** Anonymous failures force the reader to correlate line numbers with test data; named subtests state the intent directly.
- **Edge-case coverage prevents regressions.** Digest-only and tag-plus-digest references are standard in Replicated `.airgap` bundles. Missing them in tests means bugs survive review.

## When to Apply

- Any Go code that parses, normalizes, or decomposes Docker/OCI image references
- Any table-driven Go test that validates string-parsing logic
- Refactors that consolidate duplicated parsing helpers into shared packages

## Examples

### Before: digest preserved, unnamed subtests

```go
func ImageName(repo string) string {
	// BUG: does not strip digest
	if !strings.Contains(repo, "@") {
		if idx := strings.LastIndex(repo, ":"); idx > strings.LastIndex(repo, "/") {
			repo = repo[:idx]
		}
	}
	idx := strings.LastIndex(repo, "/")
	if idx == -1 {
		return repo
	}
	return repo[idx+1:]
}

func TestImageName(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"library/nginx", "nginx"},
		{"registry.com/ns/app:1.0", "app"},
	}

	for _, tt := range tests {
		// Anonymous subtest: failure only shows input/output
		got := ImageName(tt.ref)
		if got != tt.want {
			t.Errorf("ImageName(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}
```

### After: digest stripped, named subtests, full edge-case coverage

```go
func ImageName(repo string) string {
	if idx := strings.LastIndex(repo, "@"); idx != -1 {
		repo = repo[:idx]
	}
	if idx := strings.LastIndex(repo, ":"); idx > strings.LastIndex(repo, "/") {
		repo = repo[:idx]
	}
	idx := strings.LastIndex(repo, "/")
	if idx == -1 {
		return repo
	}
	return repo[idx+1:]
}

func TestImageName(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"library path", "library/nginx", "nginx"},
		{"full ref with tag", "registry.com/ns/app:1.0", "app"},
		{"plain name", "nginx", "nginx"},
		{"empty", "", ""},
		{"digest ref", "docker.io/library/nginx@sha256:abc123", "nginx"},
		{"tag and digest", "registry.com/ns/app:1.0@sha256:abc123", "app"},
		{"port registry digest", "localhost:5000/nginx@sha256:abc123", "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ImageName(tt.ref)
			if got != tt.want {
				t.Errorf("ImageName(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}
```

## Related

- `docs/solutions/best-practices/chart-image-reference-extraction-2026-05-14.md` — overlapping area (image references) focused on Helm chart extraction heuristics rather than low-level parsing
- `pkg/distribution/reference.go` — `ImageName`, `ImagePath`, and `StripTag` implementations
- `pkg/distribution/reference_test.go` — consolidated table-driven tests
- Implementation commit: `d429462`
- Review-fixes commit: `497005c`

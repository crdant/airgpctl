---
title: VHS GIF demo robustness fixes from code review
date: 2026-05-15
category: developer-experience
module: demo
problem_type: developer_experience
component: development_workflow
severity: low
applies_when:
  - Creating fixture generators or demo scripts
  - Recording terminal demos with VHS or similar tools
  - Maintaining Makefile targets for demo workflows
tags:
  - demo
  - vhs
  - error-handling
  - makefile
  - gitignore
---

# VHS GIF demo robustness fixes from code review

## Context

The airgapctl project includes a VHS-powered terminal demo that generates a GIF showcasing the CLI's core commands (`list`, `push`, `values`). During code review, several robustness and maintainability issues were identified across the demo infrastructure: improper error handling in the Go fixture generator, incomplete Makefile dependency chains, stale `.gitignore` entries, and a brittle push step in the VHS tape that could fail against registries with untrusted TLS. This learning captures the five categories of fixes applied to make the demo production-grade.

## Guidance

### 1. Fixture generators must propagate Close() errors and avoid panic()

The `demo/generate-bundle.go` script constructs a gzipped tar of Docker v2 layout blobs. Two patterns were corrected:

- **Deferred Close() → explicit error-propagating Close()**: In a script whose entire purpose is to write a file to disk, ignoring `Close()` errors means a partially-written or corrupted bundle can go undetected. The fix replaced `defer f.Close()` with sequential explicit closes that propagate errors back to `run()`.

- **panic() → error return**: `makeSingleArchManifest` originally returned `[]byte` and panicked on `json.Marshal` failure. It now returns `([]byte, error)`, letting the caller surface the problem cleanly.

### 2. Makefile targets must fail fast and depend on the right prerequisites

The `demo-deps` target previously printed missing-tool messages but did not exit with an error, which allowed the `demo` target to proceed and fail cryptically later. `exit 1` was added after each missing-dependency message.

The `demo` target was also missing explicit dependencies. It now depends on `build` and `demo-fixtures`, so `make demo` is self-contained and idempotent.

### 3. `demo-clean` and `.gitignore` must stay in sync with reality

As the demo evolved, generated artifacts changed, but cleanup rules and ignore lists drifted:

- `demo-clean` gained `demo/values-airgap.yaml` and removed stale binary paths (`demo/airgapctl`, `demo/generate-bundle`).
- `.gitignore` added `demo/values-airgap.yaml` and removed the same stale binary paths.

### 4. Demo tape scripts should tolerate real-world registry TLS

The `push` command in `demo/airgapctl-demo.tape` targets `ttl.sh`, an ephemeral registry that may present certificates not trusted by the local CA store. Adding `--tls-skip-verify` prevents the VHS recording from failing mid-tape due to TLS handshake errors.

### 5. Shell runner scripts should delegate build steps to Make

`demo/run-demo.sh` originally contained inline build and fixture-generation logic. These steps were removed and promoted to Makefile dependencies. The shell script now focuses solely on orchestrating `vhs`, while Make handles prerequisite ordering.

## Why This Matters

Demos are often treated as "throwaway" code, but they are high-visibility artifacts. A broken demo during a sales call, conference talk, or README screenshot refresh erodes trust in the tool itself. These fixes ensure:

- **No silent corruption**: A failing `Close()` or `json.Marshal()` is surfaced immediately, not swallowed.
- **Reproducible recordings**: `make demo` works on a clean checkout because every prerequisite is declared.
- **No artifact leaks**: Generated files are cleaned and ignored, keeping the working directory predictable.
- **Resilient against external flakiness**: `--tls-skip-verify` defends against ephemeral registry behavior without compromising the core tool (the flag is only in the demo tape, not production defaults).

## When to Apply

- Whenever you add a new generated artifact to a demo or fixture workflow, update **both** the Makefile `clean` target and `.gitignore` in the same commit.
- When writing small Go utilities that produce files, treat `Close()` errors as first-class citizens; `defer` is convenient but wrong if the error matters.
- When a Makefile target wraps a tool (like `vhs`), add a `*-deps` target that exits non-zero if prerequisites are missing, rather than printing advice and continuing.
- When demo scripts interact with third-party services (registries, APIs), add resilience flags that are localized to the demo context.

## Examples

### Explicit Close() chain with error propagation

```go
// Before — defer swallows errors; panics crash the process
func run() error {
    f, _ := os.Create("bundle.airgap")
    defer f.Close()
    gw := gzip.NewWriter(f)
    defer gw.Close()
    w := tar.NewWriter(gw)
    defer w.Close()
    // ... write tar contents ...
    return nil
}

// After — each close is checked and its error propagated
func run() error {
    f, err := os.Create("bundle.airgap")
    if err != nil {
        return err
    }
    gw := gzip.NewWriter(f)
    w := tar.NewWriter(gw)

    // ... write tar contents, propagate write errors ...

    if err := w.Close(); err != nil {
        _ = gw.Close()
        _ = f.Close()
        return fmt.Errorf("closing tar writer: %w", err)
    }
    if err := gw.Close(); err != nil {
        _ = f.Close()
        return fmt.Errorf("closing gzip writer: %w", err)
    }
    if err := f.Close(); err != nil {
        return fmt.Errorf("closing bundle file: %w", err)
    }
    return nil
}
```

### Makefile dependency guard

```makefile
# Before — advice printed but make continued
demo-deps:
	@if ! command -v vhs >/dev/null 2>&1; then \
		echo "vhs is missing. Install with: brew install charmbracelet/tap/vhs"; \
	fi

# After — exit 1 stops the build immediately
demo-deps:
	@if ! command -v vhs >/dev/null 2>&1; then \
		echo "vhs is missing. Install with: brew install charmbracelet/tap/vhs"; \
		exit 1; \
	fi
```

### Self-contained demo target

```makefile
# Before — runner script handled prerequisites inline
demo:
	@demo/run-demo.sh

# After — Make owns the dependency graph
demo: demo-deps build demo-fixtures
	@demo/run-demo.sh
```

## Related

- `demo/generate-bundle.go` — Go fixture generator
- `demo/airgapctl-demo.tape` — VHS tape script
- `demo/run-demo.sh` — Demo orchestration shell script
- `Makefile` — Build and demo targets
- `.gitignore` — Ignore rules for generated artifacts

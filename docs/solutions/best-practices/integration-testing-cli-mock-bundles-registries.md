---
title: "Integration testing CLI commands with mock airgap bundles and registry servers"
date: 2026-05-14
category: best-practices
module: cli
problem_type: best_practice
component: testing_framework
severity: medium
applies_when:
  - "Writing integration tests for CLI commands that cross multiple package boundaries"
  - "Testing CLI commands that require valid tar archives as input"
  - "Mocking external HTTP services (registries, APIs) in Go tests"
tags:
  - integration-testing
  - cli
  - mock-bundles
  - httptest
  - archive-tar
  - red-green-refactor
---

# Integration testing CLI commands with mock airgap bundles and registry servers

## Context

When implementing the `list`, `push`, and `values` subcommands for `airgapctl`, we needed integration tests that exercised the full command stack: Cobra flag parsing → bundle extraction → distribution layout traversal → registry push or values generation. A key challenge was that the real implementation opens `.airgap` bundles as actual `archive/tar` archives and pushes images to real OCI registries, but the early tests (written before the real implementation existed) used fake files that weren't valid tars. Once the real bundle-opening logic landed, those tests broke because `bundle.OpenBundle` expected a real tar archive with a valid `airgap.yaml` entry.

## Guidance

### 1. Use `archive/tar` to build valid mock bundles in tests

Do not write fake bundle files with `os.WriteFile(tmpFile, []byte("fake"), 0644)` once the real code expects a tar archive. Instead, create a test helper that writes a proper tar containing `airgap.yaml` and the Docker distribution v2 layout entries your code will traverse:

```go
func createMockAirgapBundle(t *testing.T, dir string, images []string, layouts []mockImageLayout) string {
    t.Helper()
    bundlePath := filepath.Join(dir, "test.airgap")
    f, err := os.Create(bundlePath)
    if err != nil {
        t.Fatalf("creating mock bundle: %v", err)
    }
    defer f.Close()

    w := tar.NewWriter(f)
    defer w.Close()

    // Write airgap.yaml
    yamlContent := fmt.Sprintf(`Version: "1"\nType: "airgap"\nSavedImages:\n%s\n`, formatSavedImages(images))
    writeTarFile(t, w, "airgap.yaml", []byte(yamlContent))

    // Write distribution layout for each image
    for _, layout := range layouts {
        writeDistributionLayout(t, w, layout)
    }

    return bundlePath
}
```

This helper keeps test setup declarative: callers specify the image list and layout metadata, and the helper handles tar serialization.

### 2. Use `httptest.Server` to mock registry v2 API endpoints

For the `push` subcommand, stand up an `httptest.NewServer` that implements the minimal Docker Registry HTTP API V2 surface your pusher needs. Record which endpoints were hit so tests can assert behavior:

```go
uploaded := make(map[string]bool)
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    switch {
    case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/blobs/"):
        w.WriteHeader(http.StatusNotFound) // blob does not exist yet
    case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/blobs/uploads/"):
        w.Header().Set("Location", "/upload")
        w.WriteHeader(http.StatusAccepted)
    case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/upload"):
        w.WriteHeader(http.StatusCreated)
    case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/"):
        w.WriteHeader(http.StatusNotFound) // manifest does not exist yet
    case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/"):
        uploaded[r.URL.Path] = true
        w.WriteHeader(http.StatusCreated)
    default:
        w.WriteHeader(http.StatusNotFound)
    }
}))
defer server.Close()
```

### 3. Prefer `Execute()` in integration tests, `ExecuteC()` in unit tests

- Use `root.Execute()` when testing the full command output (integration tests for `list`, `push`, `values`). It runs the complete pipeline and lets you assert on stdout.
- Use `ExecuteC()` when testing validation, error paths, or flag parsing in isolation. It returns the `*cobra.Command` and the error, making assertions on error messages easier without relying on side effects.

### 4. Separate unit tests (validation) from integration tests (end-to-end)

Keep unit tests in `root_test.go` that verify flag parsing, pre-run validation, and error messages with minimal or no bundle content. Keep integration tests in `commands_test.go` that construct full mock bundles and exercise the complete `RunE` pipeline. This separation makes failures faster to diagnose:

- `root_test.go`: `TestPushRequiresRegistry`, `TestListFlagWinsOverPositional`, `TestDockerConfigFallback`
- `commands_test.go`: `TestListCommand`, `TestPushCommand`, `TestValuesCommand`

### 5. Write failing tests first (RED-GREEN-REFACTOR)

Before implementing the `RunE` body of a new subcommand, write the integration test that calls the command with a mock bundle and asserts on the expected output. The test will fail because the command is unimplemented or stubbed — this confirms the test is wired correctly. Then implement the minimum code to make the test pass (GREEN). Finally, refactor with confidence.

## Why This Matters

- **Tests that survive implementation**: Fake files work when the code under test is also a fake. Once real logic is wired in, only realistic test fixtures remain valid. Building proper tar archives up front prevents a wave of test breakage when the implementation "gets real."
- **Fast, deterministic registry tests**: `httptest.Server` runs in-process with no network I/O to external services. Tests remain hermetic, parallel-safe, and fast — critical for a CI suite that validates registry push logic.
- **Confidence in cross-package integration**: CLI commands in `cmd/airgapctl/` delegate to `pkg/bundle`, `pkg/distribution`, `pkg/registry`, and `pkg/values`. Integration tests that exercise all layers together catch wiring mistakes (wrong argument order, nil pointer passes, incorrect option struct population) that unit tests in isolation miss.

## When to Apply

- Adding a new CLI subcommand that reads structured files (tar, zip, archives)
- Testing commands that interact with HTTP APIs (registries, CDNs, metadata services)
- Refactoring from stubs to real implementations where test fixtures must upgrade alongside the code
- Working in a codebase where `cmd/` delegates to multiple `pkg/` packages and cross-package integration is the primary risk

## Examples

### Before (fake bundle file that breaks once real tar parsing lands)

```go
func TestPushCommand(t *testing.T) {
    tmpFile := filepath.Join(t.TempDir(), "fake.airgap")
    os.WriteFile(tmpFile, []byte("fake"), 0644) // not a tar — breaks OpenBundle

    root := newRootCmd()
    root.SetArgs([]string{"push", "--bundle", tmpFile, "--registry", "..."})
    // ... test will fail with tar header error once real implementation exists
}
```

### After (valid mock bundle + httptest registry)

```go
func TestPushCommand(t *testing.T) {
    uploaded := make(map[string]bool)
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // ... minimal registry v2 API simulation ...
    }))
    defer server.Close()

    tmpDir := t.TempDir()
    manifestJSON := makeSingleArchManifest(t)
    bundlePath := createMockAirgapBundle(t, tmpDir, []string{"nginx:latest"}, []mockImageLayout{
        {repo: "library/nginx", tag: "latest", manifestDigest: "nginx123", manifestJSON: manifestJSON},
    })

    root := newRootCmd()
    root.SetArgs([]string{
        "push",
        "--bundle", bundlePath,
        "--registry", server.URL,
        "--username", "testuser",
        "--password", "testpass",
        "--tls-skip-verify",
    })

    var out bytes.Buffer
    root.SetOut(&out)
    root.SetErr(&out)

    if err := root.Execute(); err != nil {
        t.Fatalf("push command failed: %v\noutput: %s", err, out.String())
    }

    output := out.String()
    if !strings.Contains(output, "1 pushed") {
        t.Errorf("expected output to show pushed count, got:\n%s", output)
    }
}
```

## Related

- [Cobra CLI scaffolding with test-first flag validation and credential fallback](cobra-cli-scaffolding.md) — covers unit-test patterns for flag parsing and pre-run validation
- `cmd/airgapctl/commands_test.go` — integration tests using mock bundles and `httptest`
- `cmd/airgapctl/root_test.go` — unit tests for validation, credential fallback, and help output
- Plan: `docs/plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md`

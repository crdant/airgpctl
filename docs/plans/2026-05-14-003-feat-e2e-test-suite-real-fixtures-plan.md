---
title: E2E Test Suite with Real Fixture Bundles
type: feat
status: active
date: 2026-05-14
origin: docs/brainstorms/2026-05-14-airgapctl-requirements.md
---

# E2E Test Suite with Real Fixture Bundles

## Summary

Create an end-to-end test suite that compiles the `airgapctl` binary and exercises it against real Replicated `.airgap` bundles and Helm charts via `os/exec`. This complements the existing in-process unit and integration tests by proving the actual CLI artifact works with production bundle data.

---

## Problem Frame

All current tests execute `airgapctl` commands in-process through Cobra and use small mock bundles constructed in memory. This validates code paths but does not prove the compiled binary behaves correctly with real Replicated airgap bundles containing hundreds of megabytes of actual Docker distribution v2 layout data. An E2E layer running the real binary against real fixtures closes that gap before release.

---

## Requirements

- R1. Build the `airgapctl` binary as a pre-test step so E2E tests exercise the compiled artifact, not the test runner's in-process code.
- R2. Run `airgapctl list` against the real `/tmp/airgap/slackernews.airgap` bundle and verify it discovers and reports all images from `airgap.yaml`.
- R3. Run `airgapctl values` against the real bundle and the real slackernews chart in **all three reference modes** — local directory, local tarball, and `oci://` (via `ttl.sh`) — verifying the output YAML contains mapped image entries regardless of how the chart is supplied.
- R4. Run `airgapctl push` against the real bundle using **both** an `httptest` mock registry (fast, hermetic) and `ttl.sh` (real Docker registry over TLS), verifying the binary connects, authenticates, and reports per-image push status in both environments.
- R5. Make fixture paths configurable via environment variables so the suite can run in CI or other environments where the fixtures live elsewhere.
- R6. Isolate E2E tests behind a build tag or separate package so `go test ./...` remains fast and E2E tests run only when explicitly invoked.

**Origin actors:** A1 (SRE / Platform Engineer), A2 (airgapctl)
**Origin flows:** F1 (Load bundle into registry + generate values), F2 (Generate values only)
**Origin acceptance examples:** AE1, AE2, AE3, AE4, AE5

---

## Scope Boundaries

- Mock registry tests are hermetic and require no network; `ttl.sh` tests require outbound HTTPS but no authentication or persistent state.
- No new CLI flags or binary behavior changes — this plan is test-only.
- No permanent fixture storage in the repo (fixtures stay at their existing external paths).
- No benchmark or performance tests.

---

## Context & Research

### Relevant Code and Patterns

- `cmd/airgapctl/main.go` — entrypoint that calls `newRootCmd().Execute()`; binary exits non-zero on error.
- `cmd/airgapctl/root_test.go` — existing tests use `executeCommand(cmd, args...)` which runs Cobra in-process. E2E tests will replace this with `os/exec.Command(binPath, args...)`.
- `cmd/airgapctl/commands_test.go` — `createMockAirgapBundle()` creates tiny fake bundles. E2E tests will use the real 342 MB `slackernews.airgap` instead.
- `pkg/registry/registry_test.go` — `mockRegistry` (httptest-based) can be reused as the push target for E2E tests.
- `Makefile` — `make build` produces `./bin/airgapctl`. E2E harness can invoke this or run `go build` directly.

### Institutional Learnings

- `docs/solutions/best-practices/integration-testing-cli-mock-bundles-registries.md` — documents current mock-based approach; E2E plan extends this with real fixtures.

### External References

- Go `testing` package `TestMain` hook for one-time binary builds: https://pkg.go.dev/testing

---

## Key Technical Decisions

- **Separate `tests/e2e/` package** (not `_test` inside `cmd/airgapctl`): keeps E2E tests isolated, avoids import cycles, and makes the separate build target obvious.
- **Environment-variable fixture paths with hardcoded defaults**: `AIRGAPCTL_E2E_BUNDLE` defaults to `/tmp/airgap/slackernews.airgap`; `AIRGAPCTL_E2E_CHART` defaults to `~/sandbox/slackernews/slackernews-1.3.6-build.2.tgz`. This lets CI override without code changes.
- **Binary built once in `TestMain`**: `m.Run()` is invoked after a `go build -o tests/e2e/airgapctl ./cmd/airgapctl` step. Tests skip individually if the binary or fixtures are missing.
- **Dual push test strategy**: mock `httptest` registry for fast, hermetic, offline E2E; `ttl.sh` for real-registry verification over TLS. Both are exercised against the same real bundle. Tests that require network skip gracefully when `ttl.sh` is unreachable.
- **All three chart reference modes tested**: local directory, local tarball, and `oci://ttl.sh/...` (requires `helm` binary to pull). The OCI test pushes the chart tarball to `ttl.sh` first, then references it via `oci://` so the `helm pull` path is exercised.
- **Every CLI argument tested**: each command's flag set is exercised in at least one E2E scenario (e.g., `--verbose`, `--template`, `--version`, `--tls-ca-cert`, `--token`, `--namespace`, `--output`).

---

## Open Questions

### Resolved During Planning

- **Should push E2E use a live registry?** No — `httptest` mock registry is sufficient and avoids Docker dependency in test environments. The real bundle's blobs are read and sent to the mock, proving the binary's push path works.
- **Should fixtures be copied into the repo?** No — they are large (342 MB bundle) and already live at stable external paths. Environment variables make the suite portable.
- **How to handle slow E2E tests?** Separate `tests/e2e` package run via `make e2e` or `go test ./tests/e2e/...`. Not included in default `make test`.

### Deferred to Implementation

- **Exact output string matching vs. loose assertions**: Decided during implementation based on how stable the real bundle's output is.
- **Whether to parallelize list/values/push E2E tests**: Sequential is safer given shared binary build; parallel only if `t.Parallel()` does not race on the binary file.

---

## Output Structure

```
tests/
└── e2e/
    ├── e2e_test.go       # TestMain, binary build, shared harness
    ├── list_test.go      # E2E: airgapctl list (all args)
    ├── values_test.go    # E2E: airgapctl values (all args, all chart modes)
    ├── push_test.go      # E2E: airgapctl push (all args, mock + ttl.sh)
    └── helpers_test.go   # Shared E2E helpers (run binary, assert output, ttl.sh ping)
```

---

## Implementation Units

### U1. E2E Test Harness and Binary Builder

**Goal:** Create the shared E2E infrastructure: one-time binary compilation, fixture path resolution, and helpers for running the CLI and asserting on output.

**Requirements:** R1, R5

**Dependencies:** None

**Files:**
- Create: `tests/e2e/e2e_test.go`
- Modify: `Makefile`
- Modify: `.gitignore`

**Approach:**
- Add `tests/e2e/e2e_test.go` with `TestMain(m *testing.M)` that builds `./bin/airgapctl` (or `tests/e2e/airgapctl`) via `go build` before calling `m.Run()`.
- Provide `runAirgapctl(t, args ...string) (stdout, stderr string, exitCode int)` helper that uses `os/exec` to run the compiled binary with `t.Log` for stdout/stderr.
- Provide `fixturePath(envVar, defaultPath string) string` helper that reads env vars with fallback defaults.
- Add `make e2e` target that runs `go test -v ./tests/e2e/...`.
- Ensure default `make test` does NOT include E2E tests.
- Add `tests/e2e/airgapctl` to `.gitignore`.

**Patterns to follow:**
- `cmd/airgapctl/root_test.go` for assertion style (but using real exec instead of in-process).

**Test scenarios:**
- Happy path: `TestMain` builds binary successfully and `runAirgapctl` executes `airgapctl --help`, returning help text and exit code 0.
- Edge case: `runAirgapctl` with invalid flag returns non-zero exit code and error text in stderr.

**Verification:**
- `make e2e` compiles and runs at least one E2E test successfully.
- `make test` completes without running E2E tests.

---

### U2. List Command E2E Test with Real Bundle (All Arguments)

**Goal:** Exercise `airgapctl list` with all supported arguments against the real bundle.

**Requirements:** R2

**Dependencies:** U1

**Files:**
- Create: `tests/e2e/list_test.go`

**Approach:**
- Skip test if `AIRGAPCTL_E2E_BUNDLE` fixture is missing (`t.Skip`).
- Test both `--bundle <path>` flag and positional bundle argument.
- Test with `--verbose` flag.
- Assert stdout contains expected image names from the bundle's `airgap.yaml` (`nginx:1.29.0`, `postgres:16`, `replicated-sdk-image:1.19.3`, and the slackernews-web image).
- Assert exit code is 0 and stderr is empty (or contains only expected progress).

**Patterns to follow:**
- `cmd/airgapctl/commands_test.go` `TestListCommand` for assertion style, adapted for real exec.

**Test scenarios:**
- Happy path: `list --bundle <fixture>` prints all 4 saved images with their digests.
- Happy path: `list <fixture>` (positional arg) works identically to `--bundle`.
- Happy path: `list --bundle <fixture> --verbose` prints additional progress/diagnostic output without failing.
- Error path: `list --bundle /nonexistent` returns non-zero exit code and mentions the path in stderr.
- Error path: `list` (no bundle arg) returns non-zero exit code mentioning bundle requirement.

**Verification:**
- `go test ./tests/e2e/... -run TestList` passes when fixture is present.

---

### U3. Values Command E2E Test with Real Bundle and Chart (All Arguments & Reference Modes)

**Goal:** Exercise `airgapctl values` with all supported arguments and all three chart reference modes: local directory, local tarball, and `oci://`.

**Requirements:** R3

**Dependencies:** U1

**Files:**
- Create: `tests/e2e/values_test.go`

**Approach:**
- Skip test if bundle or chart fixture is missing.
- **Local tarball mode**: `values --bundle <fixture> --chart <tarball> --registry reg.example.com --namespace myapp --output <tmp>/values.yaml`
- **Local directory mode**: extract tarball to temp dir first, then `values --bundle <fixture> --chart <dir> --registry reg.example.com --namespace myapp --output <tmp>/values.yaml`
- **OCI mode**: push chart tarball to `ttl.sh` (e.g., `helm push` or `oras push`), then `values --bundle <fixture> --chart oci://ttl.sh/<name> --version 1.3.6 --registry reg.example.com --namespace myapp --output <tmp>/values.yaml`
- Test `--template` flag with a custom template string.
- Assert exit code is 0 for all modes.
- Read generated values file and assert it contains `reg.example.com` and `myapp`.
- Assert at least one expected image mapping is present (e.g., `nginx` mapped to the registry namespace path).

**Patterns to follow:**
- `cmd/airgapctl/commands_test.go` `TestValuesCommand` for assertion style.
- `pkg/values/values_test.go` for values file content assertions.

**Test scenarios:**
- Happy path: values generation with real bundle and **local tarball** chart produces valid YAML.
- Happy path: values generation with real bundle and **local directory** chart produces identical YAML to tarball mode.
- Happy path: values generation with real bundle and **OCI chart reference** (`oci://ttl.sh/...`) produces valid YAML, proving `helm pull` integration works.
- Happy path: values generation with **custom `--template`** produces correctly remapped image paths.
- Edge case: `values --bundle <fixture>` (missing `--chart`) returns non-zero exit code mentioning chart requirement.
- Edge case: `values --bundle <fixture> --chart oci://ttl.sh/...` (missing `--version`) returns non-zero or uses latest tag.

**Verification:**
- `go test ./tests/e2e/... -run TestValues` passes when fixtures and network are present.

---

### U4. Push Command E2E Test with Real Bundle — Mock Registry & ttl.sh

**Goal:** Exercise `airgapctl push` with all supported arguments against both a local mock registry and the real `ttl.sh` ephemeral registry.

**Requirements:** R4

**Dependencies:** U1

**Files:**
- Create: `tests/e2e/push_test.go`

**Approach:**

**Mock registry tests (hermetic, no network):**
- Start an `httptest.Server` implementing Docker Registry HTTP API V2.
- Test all auth variants:
  - `--username <u> --password <p>`
  - `--token <t>` (bearer token)
  - `--tls-skip-verify`
  - `--tls-ca-cert <path>` (mock server with custom CA)
- Run `airgapctl push --bundle <fixture> --registry <mock-url> <auth-args>`.
- Assert exit code is 0 and stdout contains progress + summary.

**ttl.sh tests (real registry, requires network):**
- Skip if `ttl.sh` is unreachable (`t.Skip` after a quick ping).
- Run `airgapctl push --bundle <fixture> --registry ttl.sh --namespace <random> --tls-skip-verify`.
- `ttl.sh` does not require auth, so test without `--username`/`--password`.
- Assert exit code is 0 and images are pullable from `ttl.sh/<namespace>/<image>:<tag>`.
- Use a unique namespace per test run to avoid collisions.

**Test scenarios:**
- Happy path: push to **mock registry** with `--username/--password` succeeds.
- Happy path: push to **mock registry** with `--token` succeeds.
- Happy path: push to **mock registry** with `--tls-ca-cert` succeeds.
- Happy path: push to **ttl.sh** with `--namespace` succeeds and images are pullable.
- Error path: push without `--registry` returns non-zero exit code.
- Error path: push with invalid `--username/--password` to mock registry returns auth failure.
- Error path: push to unreachable registry returns non-zero exit code with network error.

**Verification:**
- `go test ./tests/e2e/... -run TestPush` passes.
- Mock registry logs show blob + manifest requests.
- `ttl.sh` images can be verified with a `docker pull` or `crane manifest` check.

---

## System-Wide Impact

- **Interaction graph:** E2E tests are a new top-level `tests/e2e` package that imports nothing from `cmd/airgapctl` (they shell out). Zero coupling risk.
- **Error propagation:** E2E tests assert on exit codes and stderr text. Changes to error message formatting in the CLI could break E2E assertions — acceptable trade-off for E2E coverage.
- **State lifecycle risks:** E2E tests write temporary values files via `--output`. Each test must use `t.TempDir()` for output paths to avoid collisions.
- **Unchanged invariants:** Existing unit and integration tests in `cmd/airgapctl/*_test.go` and `pkg/*/*_test.go` are untouched and continue to run via `go test ./...`.

---

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Fixture paths change or fixtures are missing on developer machines | Environment variables with defaults; tests skip gracefully with `t.Skip` instead of failing |
| E2E tests are slow due to 342 MB bundle parsing | Run E2E separately from unit tests (`make e2e`); do not include in default `make test` |
| Real bundle content changes and breaks output assertions | Use loose assertions (contains, not exact match) for image names; avoid asserting on exact digests or progress timestamps |
| Binary build fails in TestMain | Fail fast in `TestMain` with clear log output so all tests are skipped |

---

## Documentation / Operational Notes

- Update `README.md` (or `docs/` if present) to document `make e2e` and required fixture paths.
- Add `AIRGAPCTL_E2E_BUNDLE` and `AIRGAPCTL_E2E_CHART` environment variables to CI documentation if applicable.

---

## Sources & References

- **Origin document:** [docs/brainstorms/2026-05-14-airgapctl-requirements.md](docs/brainstorms/2026-05-14-airgapctl-requirements.md)
- Related code: `cmd/airgapctl/commands_test.go`, `pkg/registry/registry_test.go`
- Real fixtures: `/tmp/airgap/slackernews.airgap` (342 MB Replicated airgap bundle), `~/sandbox/slackernews/slackernews-1.3.6-build.2.tgz` (Helm chart)

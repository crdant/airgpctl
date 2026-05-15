---
title: "fix: Review findings — auth, error handling, parsing robustness, and test coverage"
type: fix
status: active
date: 2026-05-14
origin: ce-code-review on current branch
---

# Review Findings Fixes

## Summary

Address the residual P1–P3 findings from the post-implementation code review of the `list`, `push`, and `values` subcommands. The issues cluster into four themes: (1) dead/unwired CLI flags and auth pathways, (2) swallowed errors and fragile string matching, (3) missing fallback behavior for edge-case inputs, and (4) test coverage gaps on new code paths introduced during the previous sprint.

---

## Problem Frame

The prior implementation sprint landed working `list`, `push`, and `values` subcommands validated against a real `.airgap` bundle. A structured review surfaced specific defects that do not block basic operation but will bite users in production or CI: bearer-token auth is advertised but not wired, docker-config parse failures are silently ignored, chart-value image detection uses naive substring matching that misfires on similarly-named keys, and several new helpers have zero test coverage.

---

## Scope Boundaries

### In Scope
- Wire `push --token` / `AIRGAPCTL_REGISTRY_TOKEN` through to registry HTTP transport as bearer auth.
- Surface `loadDockerConfig()` errors instead of discarding them.
- Harden `extractImageRefs` against false-positive key matches.
- Deduplicate `imagePath` logic between `cmd/airgapctl/push.go` and `pkg/values/values.go`.
- Graceful fallback when `.airgap` is a plain tar (non-gzipped).
- Safer directory selection after `helm pull --untar` or tarball extraction.
- Treat registry `401 Unauthorized` as a retriable/auth-failure rather than a fatal push abort.
- Add unit tests for `RemapChartValues`, `extractTarball`, `imagePath`, `findRepo`, and HTTPS scheme normalization.
- Correct error message capitalization to match the actual YAML field name.

### Deferred to Follow-Up Work
- Full OAuth2 / identity-token refresh flows for registries that require it.
- Helm chart dependency (`charts/`) directory handling during `--untar`.
- Fuzz testing of `extractImageRefs` against arbitrary third-party chart YAML.

### Outside Scope
- New features (e.g., `--template` re-enablement, additional output formats).
- Refactoring of the registry push engine to use `go-containerregistry` instead of the current pure-Go HTTP implementation.

---

## Requirements

- R1. `--token` and `AIRGAPCTL_REGISTRY_TOKEN` must authenticate registry pushes when username/password are absent. (Fixes dead code.)
- R2. Malformed `~/.docker/config.json` must produce a visible error instead of being silently ignored. (Fixes swallowed error.)
- R3. Chart values parsing must not match keys like `some_repository:` or `myregistry:` as image references. (Fixes false positives.)
- R4. `imagePath` must live in a single shared location consumed by both `push` and `values`. (Fixes duplication.)
- R5. `.airgap` bundles that are plain tar archives must extract successfully. (Fixes missing fallback.)
- R6. After `helm pull --untar`, the code must locate the correct chart directory even when multiple directories exist. (Fixes naive first-directory selection.)
- R7. Registry `401 Unauthorized` on manifest/blob HEAD checks must be reported as an auth failure, not an unexpected fatal error that aborts the entire push. (Fixes overly broad error classification.)
- R8. All new public functions introduced since U5/U6 must have unit tests. (Fixes coverage gaps.)

---

## Key Technical Decisions

- **Bearer auth transport**: Add a `Token` field to `registry.Config`. When non-empty, `setAuth` sets `Authorization: Bearer <token>` instead of basic auth. This is the standard OCI registry token mechanism and aligns with the already-present CLI flag.
- **Docker config error handling**: Return the `loadDockerConfig()` error from `PreRunE` so Cobra prints it before push starts. Do not fallback to anonymous push when the config file is present but unreadable — that masks credential problems.
- **Chart key matching**: Replace `strings.Index(line, "repository:")` with a regex or line-prefix check that requires `repository:` at the start of a YAML key (after optional whitespace), not as an arbitrary substring.
- **Shared `imagePath`**: Move `imagePath`, `imageName`, and `stripTag` to `pkg/distribution/reference.go` (new file) as pure string utilities. Both `cmd/airgapctl/push.go` and `pkg/values/values.go` import them. This keeps repository-path logic colocated with the distribution package that already owns image reference parsing.
- **Tar/gzip detection**: In `OpenBundle` and `ExtractTo`, attempt `gzip.NewReader` first. If that returns `gzip.ErrHeader`, rewind the file handle and retry as plain tar. This preserves backward compatibility with any non-gzipped legacy bundles.
- **Directory selection after extraction**: After `helm pull --untar`, read `Chart.yaml` from each immediate subdirectory and match `metadata.name` to the chart reference name (last path component of the OCI URL). Fallback to the first directory only if no `Chart.yaml` match is found.
- **Registry HTTP status handling**: In `manifestExists` and `pushBlob` HEAD checks, treat `401` and `403` as distinct auth errors. Treat `500`/`502`/`503` as retriable. Keep `404` as the "does not exist, proceed" signal. Return wrapped errors so the caller can classify and report them per-image.

---

## Implementation Units

### U1. Wire bearer-token auth and surface docker-config errors

**Goal:** Make `--token` and `AIRGAPCTL_REGISTRY_TOKEN` functional, and stop swallowing `loadDockerConfig()` errors.

**Requirements:** R1, R2

**Dependencies:** None

**Files:**
- `pkg/registry/registry.go` — add `Token` to `Config`; update `setAuth` to set `Authorization: Bearer <token>` when token is present.
- `cmd/airgapctl/push.go` — forward `pushOpts.token` into `registry.Config`; capture and return `loadDockerConfig()` error in `PreRunE`.

**Approach:**
- Add `Token string` to `registry.Config` struct.
- In `setAuth`: if `cfg.Token != ""`, set `req.Header.Set("Authorization", "Bearer "+cfg.Token)`. Else fall back to existing basic auth.
- In `newPushCmd().PreRunE`: change `_ = loadDockerConfig()` to `if err := loadDockerConfig(); err != nil { return err }`.
- Pass `pushOpts.token` into `registry.Config{...}` in `RunE`.

**Test scenarios:**
- `TestPusher_Push_TokenAuth` — mock registry expects `Authorization: Bearer test-token` header; verify push succeeds.
- `TestPushCommand_Token` — create a mock bundle, run `push --registry <mock> --token testtoken`, verify command succeeds.
- `TestPush_DockerConfigParseError` — create a malformed `~/.docker/config.json`, set `DOCKER_CONFIG` to its dir, run push, verify error mentions docker config parsing.

**Verification:**
- `go test ./pkg/registry/...` passes.
- `go test ./cmd/airgapctl/...` passes.
- `./bin/airgapctl push --registry ttl.sh --token <token> --bundle ...` pushes successfully (smoke test).

---

### U2. Harden chart image reference extraction

**Goal:** Eliminate false-positive key matches in `extractImageRefs`.

**Requirements:** R3

**Dependencies:** None

**Files:**
- `cmd/airgapctl/values.go` — rewrite `extractImageRefs`.

**Approach:**
- Instead of `strings.Index(line, "repository:")`, use a key-prefix check: after trimming leading whitespace, the line must start with `repository:` (or `image:`) at the key position.
- Optionally use `strings.HasPrefix(strings.TrimSpace(line), "repository:")` for simplicity — this avoids matching `some_repository:` while still catching properly-indented YAML keys.
- Keep the existing array-item detection (`- ` prefix) unchanged; it already requires `/` and `:` which is a reasonable heuristic.

**Test scenarios:**
- `TestExtractImageRefs_NoFalsePositives` — input contains `some_repository: foo`, `myregistry: bar`, `imagePullPolicy: Always`; verify none are returned.
- `TestExtractImageRefs_RepositoryKey` — input contains `  repository: nginx` at various indentation levels; verify it is extracted.
- `TestExtractImageRefs_ImageKey` — input contains `image: "docker.io/nginx:latest"`; verify it is extracted.

**Verification:**
- `go test ./cmd/airgapctl/...` passes.

---

### U3. Deduplicate image-path helpers into `pkg/distribution`

**Goal:** Move `imagePath`, `imageName`, `stripTag` to a single shared location.

**Requirements:** R4

**Dependencies:** None

**Files:**
- Create: `pkg/distribution/reference.go`
- Create: `pkg/distribution/reference_test.go`
- Modify: `cmd/airgapctl/push.go` — remove local `imagePath`, import from `pkg/distribution`.
- Modify: `pkg/values/values.go` — remove local `imageName`, `imagePath`, `stripTag`; import from `pkg/distribution`.

**Approach:**
- Create `pkg/distribution/reference.go` with exported functions: `ImageName(ref string) string`, `ImagePath(ref string) string`, `StripTag(ref string) string`.
- Move implementations from existing locations without changing logic.
- Update all call sites to use the exported names.

**Test scenarios:**
- `TestImageName` — covers `library/nginx`, `registry.com/ns/app:1.0`, `nginx`, empty string.
- `TestImagePath` — covers `registry.com/ns/app:1.0` → `ns/app`, `library/nginx:latest` → `library/nginx`, `nginx` → `nginx`.
- `TestStripTag` — covers `nginx:latest` → `nginx`, `nginx` → `nginx`, `nginx@sha256:abc` → `nginx@sha256:abc` (digest refs should not be stripped).

**Verification:**
- `go test ./pkg/distribution/...` passes.
- `go test ./pkg/values/...` passes.
- `go test ./cmd/airgapctl/...` passes.

---

### U4. Graceful tar/gzip detection in bundle extraction

**Goal:** Support both gzipped and plain-tar `.airgap` bundles.

**Requirements:** R5

**Dependencies:** None

**Files:**
- `pkg/bundle/bundle.go` — update `OpenBundle` and `ExtractTo`.

**Approach:**
- In both functions, after opening the file, attempt `gzip.NewReader`. If it returns `gzip.ErrHeader`, `Seek(0, io.SeekStart)` the file and fall back to `tar.NewReader(f)` directly.
- This requires the file to be an `io.ReadSeeker`; `os.File` satisfies this.

**Test scenarios:**
- `TestOpenBundle_PlainTar` — create a mock `.airgap` that is a plain tar (no gzip layer) containing `airgap.yaml`; verify it opens successfully.
- `TestBundle_ExtractTo_PlainTar` — extract a plain-tar bundle and verify contents.
- `TestOpenBundle_GzippedTar` — existing test should still pass (regression guard).

**Verification:**
- `go test ./pkg/bundle/...` passes.

---

### U5. Safer chart directory selection after extraction

**Goal:** Pick the correct chart directory after `helm pull --untar` or tarball extraction, even when multiple directories exist.

**Requirements:** R6

**Dependencies:** None

**Files:**
- `cmd/airgapctl/values.go` — refactor the directory-finding logic into a helper.

**Approach:**
- Extract the "find chart directory in extracted contents" logic into a new helper `findChartDir(root string, expectedName string) (string, error)`.
- The helper reads each immediate subdirectory, looks for `Chart.yaml`, parses it for `metadata.name`, and returns the first directory whose name matches.
- If no match, fallback to the first directory (existing behavior) but log a warning when `--verbose` is set.
- For OCI charts, the expected name is the last path component of the OCI reference (e.g., `slackernews` from `oci://.../slackernews`).
- For tarball charts, the expected name is derived from the tarball filename (strip `.tgz` / `.tar.gz`).

**Test scenarios:**
- `TestFindChartDir_Match` — root contains `foo/` and `bar/`; `bar/Chart.yaml` has `name: bar`; expected name `bar`; returns `root/bar`.
- `TestFindChartDir_Fallback` — root contains `foo/` and `bar/`; neither has matching `Chart.yaml`; returns first directory.
- `TestFindChartDir_SingleDir` — root contains only `chart/`; returns it.

**Verification:**
- `go test ./cmd/airgapctl/...` passes.

---

### U6. Improve registry HTTP error classification

**Goal:** Distinguish auth failures and retriable errors from permanent "unexpected status" fatals during push.

**Requirements:** R7

**Dependencies:** U1 (token auth must be wired first so 401 can be retried with correct credentials)

**Files:**
- `pkg/registry/registry.go` — update `manifestExists` and `pushBlob`.

**Approach:**
- In `manifestExists` and `pushBlob` HEAD checks:
  - `404` → return `false, nil` (does not exist, proceed to upload).
  - `200` → return `true, nil` (already exists).
  - `401`, `403` → return `false, fmt.Errorf("registry authentication failed: %s", url)`.
  - `500`, `502`, `503`, `504` → return `false, fmt.Errorf("registry temporarily unavailable (status %d): %s", resp.StatusCode, url)`.
  - All other statuses → keep existing `fmt.Errorf("unexpected status %d ...")`.
- The caller (`pushOne`) already returns these as per-image failures, so no change needed in the orchestration layer.

**Test scenarios:**
- `TestPusher_Push_AuthFailure` — mock registry returns `401` on manifest HEAD; verify error message mentions "authentication failed".
- `TestPusher_Push_RetriableFailure` — mock registry returns `503` on blob HEAD; verify error message mentions "temporarily unavailable".
- `TestPusher_Push_UnexpectedStatus` — mock registry returns `418`; verify error message mentions "unexpected status".

**Verification:**
- `go test ./pkg/registry/...` passes.

---

### U7. Add unit-test coverage for new public functions

**Goal:** Close coverage gaps on helpers introduced in the previous sprint.

**Requirements:** R8

**Dependencies:** U3 (shared reference helpers need their own tests), U5 (`findChartDir` needs tests)

**Files:**
- `pkg/values/values_test.go` — add `TestRemapChartValues_*`.
- `pkg/distribution/distribution_test.go` — add `TestFindRepo_*`, `TestNewWalker_NestedLayout`.
- `cmd/airgapctl/values_test.go` — add `TestExtractTarball_*`, `TestIsChartTarball_*`, `TestFindChartDir_*`, `TestExtractImageRefs_*`, `TestWriteValuesYAML_*`.

**Approach:**
- Test `RemapChartValues` with:
  - Chart containing nested image definitions → verify structure preserved.
  - Chart with `releaseImages` array → verify array items remapped.
  - Chart with no matching images → verify output equals input.
- Test `findRepo` with:
  - Direct repository match.
  - Fallback to last-component match.
  - No match → error.
- Test `NewWalker` nested layout auto-detection:
  - Directory with `images/docker/registry/v2` subdirectory → walker uses nested path.
  - Directory without nested layout → walker uses root.
- Test `extractTarball`:
  - Happy path: extract a valid tgz.
  - Error path: tar entry attempts directory traversal (`../../evil`) → error.
- Test `writeValuesYAML`:
  - Writes valid YAML.
  - Creates parent directories.

**Verification:**
- `go test ./...` passes with no new uncovered public functions.
- `go test ./... -cover` shows improvement in `pkg/values`, `pkg/distribution`, and `cmd/airgapctl` packages.

---

### U8. Fix error message capitalization

**Goal:** Error message must reference the actual lowercase YAML field name.

**Requirements:** — (polish, no R-ID)

**Dependencies:** None

**Files:**
- `pkg/bundle/bundle.go` — one-line change.

**Approach:**
- Change `"airgap.yaml missing SavedImages field"` to `"airgap.yaml missing savedImages field"`.

**Verification:**
- `go test ./pkg/bundle/...` passes (test that asserts the error message may need updating).

---

## System-Wide Impact

- **Auth flow change:** U1 makes `--token` take precedence over basic auth when present. This is standard behavior and aligns with user expectations, but scripts that accidentally set `--token` alongside `--username` will now use token auth instead of basic auth. Documented in help text.
- **Error propagation change:** U1 causes malformed docker config to abort the push early (in `PreRunE`) rather than proceeding with possibly-wrong credentials. This is safer but changes behavior for users with broken docker configs.
- **No API contract changes:** All fixes are internal or CLI-facing; no exported Go API surfaces change except the addition of `Token` to `registry.Config`.

---

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Moving `imagePath` to `pkg/distribution` breaks existing tests that import it from `pkg/values` | U3 includes updating all call sites and tests in the same commit; CI will catch any missed references. |
| Plain-tar fallback accidentally accepts non-tar files | The fallback only triggers when `gzip.ErrHeader` is returned; any other gzip error (corrupt data) still propagates as an error. |
| `findChartDir` may pick wrong directory if multiple charts have same `metadata.name` | Document the fallback behavior; in practice Replicated bundles extract a single chart. |
| Registry HTTP reclassification may hide real bugs behind "temporarily unavailable" | The status codes matched (`500`, `502`, `503`, `504`) are standard retriable HTTP codes; all others remain "unexpected" fatals. |

---

## Sources & References

- **Origin review:** Findings from `ce-code-review` run on current branch (2026-05-14).
- **Requirements:** [docs/brainstorms/2026-05-14-airgapctl-requirements.md](docs/brainstorms/2026-05-14-airgapctl-requirements.md)
- **Active plan:** [docs/plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md](docs/plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md)

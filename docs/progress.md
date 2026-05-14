# airgapctl Implementation Progress

## Active Plan
[docs/plans/2026-05-14-002-fix-review-findings-plan.md](docs/plans/2026-05-14-002-fix-review-findings-plan.md)

## Implementation Units (Review Findings Fixes)

- [x] **U1. Wire bearer-token auth and surface docker-config errors**
- [x] **U2. Harden chart image reference extraction**
- [x] **U3. Deduplicate image-path helpers into pkg/distribution**
- [x] **U4. Graceful tar/gzip detection in bundle extraction**
- [x] **U5. Safer chart directory selection after extraction**
- [x] **U6. Improve registry HTTP error classification**
- [x] **U7. Add unit-test coverage for new public functions**
- [ ] **U8. Fix error message capitalization**

## Previous Plan (Completed)
[docs/plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md](docs/plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md)

- [x] **U1. Cobra CLI scaffolding and command structure**
- [x] **U2. Bundle extraction and airgap.yaml parsing**
- [x] **U3. Docker v2 layout traversal and manifest resolution**
- [x] **U4. Registry push engine with multi-arch and idempotency**
- [x] **U5. Chart-structure-preserving values file generation**
- [x] **U6. Subcommand implementations (list, push, values)**

## Notes

- go.mod exists with module path `github.com/replicatedhq/airgapctl` and Go 1.25.8
- Makefile has build, test, lint, coverage targets

## Completed

### U1 — Cobra CLI scaffolding (2026-05-14)

**Files created/modified:**
- `cmd/airgapctl/main.go` — entrypoint calling `newRootCmd().Execute()`
- `cmd/airgapctl/root.go` — root command with `--bundle` (persistent, wins over positional arg) and `--verbose` flags; validates bundle path is a file not directory
- `cmd/airgapctl/list.go` — `list` subcommand stub
- `cmd/airgapctl/push.go` — `push` subcommand with `--registry`, `--username`, `--password`, `--token`, `--tls-skip-verify`, `--tls-ca-cert`; credential resolution chain (flags → env vars → `~/.docker/config.json` with `DOCKER_CONFIG` support and base64 auth decoding)
- `cmd/airgapctl/values.go` — `values` subcommand with `--chart`, `--registry`, `--namespace`, `--output` (default `values-airgap.yaml`), `--template`
- `cmd/airgapctl/root_test.go` — comprehensive tests for command structure, flag parsing, bundle validation, credential fallback, help output
- `.gitignore` — fixed `airgapctl` → `/airgapctl` so `cmd/airgapctl/` is not ignored

**Tests:** `go test ./cmd/airgapctl/...` passes (14 tests).
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Learning documented:** `docs/solutions/best-practices/cobra-cli-scaffolding.md`

### U2 — Bundle extraction and airgap.yaml parsing (2026-05-14)

**Files created/modified:**
- `pkg/bundle/bundle.go` — `OpenBundle()` opens `.airgap` tar and parses `airgap.yaml`; `ExtractTo()` safely extracts all bundle contents with directory traversal protection
- `pkg/bundle/bundle_test.go` — table-driven tests covering happy path, edge cases (empty SavedImages), error paths (missing airgap.yaml, missing SavedImages field, directory traversal), and file-not-found

**Tests:** `go test ./pkg/bundle/...` passes (5 tests).
**Build:** `make build` succeeds.

**Learning documented:** `docs/solutions/design-patterns/safe-tar-extraction-yaml-parsing-airgap-bundles.md`

### U3 — Docker v2 layout traversal and manifest resolution (2026-05-14)

**Files created/modified:**
- `pkg/distribution/walker.go` — `Walker` traverses `images/` directory and resolves manifest lists vs single-arch manifests
- `pkg/distribution/walker_test.go` — table-driven tests covering single-arch, multi-arch, multiple repos, missing tags, invalid blobs, and fully-qualified registries

**Tests:** `go test ./pkg/distribution/...` passes.
**Build:** `make build` succeeds.

### U4 — Registry push engine with multi-arch and idempotency (2026-05-14)

**Files created/modified:**
- `pkg/registry/registry.go` — `Pusher` implements pure Go push via Docker Registry HTTP API V2
- `pkg/registry/pusher.go` — `Push()` handles single-arch and multi-arch manifest lists with correct blob→manifest→list ordering; idempotent via HEAD checks; progress callback; `[]Report` per-image status
- `pkg/registry/pusher_test.go` — table-driven tests with mock `httptest` registry covering single-arch, multi-arch, idempotency, progress, auth, and partial failure

**Tests:** `go test ./pkg/registry/...` passes.
**Build:** `make build` succeeds.

**Learning documented:** `docs/solutions/design-patterns/pure-go-registry-push-docker-v2-on-disk-layout.md`

### U5 — Chart-structure-preserving values file generation (2026-05-14)

**Files created/modified:**
- `pkg/values/generator.go` — `Generator` produces YAML values files with configurable path template (`{registry}/{namespace}/{name}:{tag}`)
- `pkg/values/generator_test.go` — 13 table-driven tests covering happy path, custom templates, tag override, filtering, chart-aware matching, empty image sets, invalid templates, duplicate detection, and helper functions

**Tests:** `go test ./pkg/values/...` passes.
**Build:** `make build` succeeds.

**Learning documented:** `docs/solutions/design-patterns/template-based-helm-values-generation-airgap-images.md`

### U6 — Subcommand implementations (list, push, values) (2026-05-14)

**Files created/modified:**
- `cmd/airgapctl/list.go` — `list` subcommand: extracts bundle, resolves manifests via `distribution.Walker`, prints tabular output with image type, digest, and platforms
- `cmd/airgapctl/push.go` — `push` subcommand: extracts bundle, resolves images, pushes to registry via `registry.Pusher` with progress reporting and per-image success/skip/failure summary
- `cmd/airgapctl/values.go` — `values` subcommand: extracts bundle, resolves images, generates values file via `values.Generator` with chart-aware filtering from chart `values.yaml`
- `cmd/airgapctl/commands_test.go` — integration tests for all three subcommands using mock airgap bundles (tar with distribution layout) and `httptest` mock registry
- `cmd/airgapctl/root_test.go` — updated credential tests to use valid mock bundles and mock registry servers

**Tests:** `go test ./cmd/airgapctl/...` passes (18 tests).
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Learning documented:** `docs/solutions/best-practices/integration-testing-cli-mock-bundles-registries.md`

---

## Review Findings Fixes (New Plan)

### U1 — Wire bearer-token auth and surface docker-config errors (2026-05-14)

**Files created/modified:**
- `pkg/registry/registry.go` — added `Token` to `Config`; `setAuth` sets `Authorization: Bearer <token>` when token is present
- `cmd/airgapctl/push.go` — forwarded `pushOpts.token` into `registry.Config`; captured and returned `loadDockerConfig()` error in `PreRunE`
- `pkg/registry/registry_test.go` — added `TestPusher_Push_TokenAuth`
- `cmd/airgapctl/commands_test.go` — added `TestPushCommand_Token` and `TestPush_DockerConfigParseError`
- `cmd/airgapctl/push_test.go` — added `TestImagePath`, `TestLoadDockerConfig_MissingFile`, `TestLoadDockerConfig_MalformedJSON`, `TestLoadDockerConfig_SchemeMatching`, `TestLoadDockerConfig_MalformedAuthField`

**Tests:** `go test ./pkg/registry/...` and `go test ./cmd/airgapctl/...` pass.
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Commits:** `c50aae3`, `09c2920`
**Learning documented:** `docs/solutions/best-practices/registry-auth-docker-config-error-handling-2026-05-14.md`

### U2 — Harden chart image reference extraction (2026-05-14)

**Files created/modified:**
- `cmd/airgapctl/values.go` — `extractImageRefs` now uses `strings.HasPrefix` for `repository:` and `image:` keys instead of substring search; handles single-quoted and unquoted values; array-item heuristic relaxed to catch Docker Hub short names; `isChartTarball` tightened to chart tarballs only; `extractTarball` skips PAX extended headers; removed dead `valuesOpts.template` field
- `cmd/airgapctl/values_test.go` — new file with `TestExtractImageRefs_NoFalsePositives`, `TestExtractImageRefs_RepositoryKey`, `TestExtractImageRefs_ImageKey`, `TestExtractImageRefs_ImageKeySingleQuoted`, `TestExtractImageRefs_ImageKeyUnquoted`, `TestExtractImageRefs_ArrayItems`, `TestExtractImageRefs_EmptyValues`, `TestIsChartTarball`, `TestExtractTarball_DirectoryTraversal`, `TestWriteValuesYAML`
- `pkg/values/values.go` — `ChartMatcher.Match` now uses exact short-name equality instead of bidirectional `strings.Contains`

**Tests:** `go test ./cmd/airgapctl/...` and `go test ./pkg/values/...` pass.
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Commits:** `1ab3483`, `50f93d2`
**Learning documented:** `docs/solutions/best-practices/chart-image-reference-extraction-2026-05-14.md`

### U3 — Deduplicate image-path helpers into pkg/distribution (2026-05-14)

**Original commit:** `d429462`

**Review findings addressed:**
- `ImageName` did not strip digest references, causing incorrect short-name extraction (e.g. `"nginx@sha256:abc123"` → `"nginx@sha256:abc123"` instead of `"nginx"`). This broke chart image matching and filtering for digest-based bundle images.
- `TestImageName` and `TestStripTag` used plain loops without `t.Run`, inconsistent with `TestImagePath`.
- Missing test coverage for digest refs, combined tag+digest, and port-registry+digest edge cases.

**Files created/modified:**
- `pkg/distribution/reference.go` — fixed `ImageName` to strip digest (and tag) before extracting last path component; removed misleading "do not strip digest" comment
- `pkg/distribution/reference_test.go` — added digest test cases for `ImageName`; added `t.Run` wrappers for `TestImageName` and `TestStripTag`; added port-registry+digest test for `ImagePath`

**Tests:** `go test ./pkg/distribution/...` passes.
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Commits:** `d429462`, `497005c`
**Learning documented:** `docs/solutions/best-practices/image-reference-parsing-testing-conventions-2026-05-14.md`

### U4 — Graceful tar/gzip detection in bundle extraction (2026-05-14)

**Files created/modified:**
- `pkg/bundle/bundle.go` — `OpenBundle` and `ExtractTo` now autodetect gzip compression via shared `openBundleFile` helper; falls back to plain tar when `gzip.ErrHeader` is returned
- `pkg/bundle/bundle_test.go` — added `TestOpenBundle_PlainTar`, `TestBundle_ExtractTo_PlainTar`, `TestOpenBundle_GzippedTar` (regression), `TestOpenBundle_EmptyFile`; merged `createMockAirgapBundle` and `createPlainTarBundle` into single `createMockBundle` helper

**Tests:** `go test ./pkg/bundle/...` passes (11 tests).
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Commits:** `d241cb9`, `cad17bf`
**Learning documented:** `docs/solutions/best-practices/dry-gzip-tar-detection-test-helpers-edge-cases-2026-05-14.md`

### U5 — Safer chart directory selection after extraction (2026-05-14)

**Files created/modified:**
- `cmd/airgapctl/values.go` — extracted duplicate "find first directory" logic into `findChartDir(root, expectedName)` helper; added `chartNameFromTarball(path)` for deriving expected name from tarball filename; updated OCI and tarball extraction call sites to use `findChartDir`
- `cmd/airgapctl/values_test.go` — added `TestFindChartDir_Match`, `TestFindChartDir_Fallback`, `TestFindChartDir_SingleDir`

**Tests:** `go test ./cmd/airgapctl/...` passes (36 tests).
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Commits:** `701f38a`, `73311cc`
**Learning documented:** `docs/solutions/best-practices/safer-chart-directory-selection-after-extraction.md`

### U6 — Improve registry HTTP error classification (2026-05-14)

**Files created/modified:**
- `pkg/registry/registry.go` — extracted `registryError(statusCode, url)` helper; classified auth failures (401/403), retriable errors (500/502/503/504), and unexpected statuses; applied classification consistently across `manifestExists`, `pushBlob` (HEAD, POST, PUT), and `pushManifest` (PUT)
- `pkg/registry/registry_test.go` — added `TestPusher_Push_AuthFailure`, `TestPusher_Push_RetriableFailure`, `TestPusher_Push_UnexpectedStatus`, `TestPusher_Push_ManifestAuthFailure`, `TestPusher_Push_ManifestRetriableFailure`, `TestPusher_Push_BlobUploadAuthFailure`

**Tests:** `go test ./pkg/registry/...` passes.
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Commits:** `34c1290`, `e3b370c`
**Learning documented:** `docs/solutions/conventions/registry-http-error-classification-2026-05-14.md`

### U7 — Add unit-test coverage for new public functions (2026-05-14)

**Files created/modified:**
- `pkg/values/values_test.go` — added `TestRemapChartValues_NestedImageDefinitions`, `TestRemapChartValues_ReleaseImagesArray`, `TestRemapChartValues_NoMatchingImages`
- `pkg/distribution/distribution_test.go` — added `TestFindRepo_DirectMatch`, `TestFindRepo_Fallback`, `TestFindRepo_NoMatch`, `TestNewWalker_NestedLayout`, `TestNewWalker_NonNestedLayout`
- `cmd/airgapctl/values_test.go` — added `TestExtractTarball_HappyPath`, `TestWriteValuesYAML_ValidYAML`, `TestWriteValuesYAML_CreatesParentDirs`

**Tests:** `go test ./...` passes.
**Build:** `go build ./cmd/airgapctl` produces working binary.

**Coverage improvements:**
- `pkg/values`: 38.4% → 85.7%
- `pkg/distribution`: 90.9% → 92.7%
- `cmd/airgapctl`: 73.6% → 76.3%

**Commits:** `210755f`, `b56192c`
**Learning documented:** `docs/solutions/best-practices/unit-test-coverage-public-functions-go-2026-05-14.md`

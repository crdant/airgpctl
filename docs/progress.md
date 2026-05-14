# airgapctl Implementation Progress

## Active Plan
[docs/plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md](docs/plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md)

## Implementation Units

- [x] **U1. Cobra CLI scaffolding and command structure**
- [x] **U2. Bundle extraction and airgap.yaml parsing**
- [ ] U3. Docker v2 layout traversal and manifest resolution
- [ ] U4. Registry push engine with multi-arch and idempotency
- [ ] U5. Chart-structure-preserving values file generation
- [ ] U6. Subcommand implementations (list, push, values)

## Notes

- go.mod exists with module path `github.com/replicatedhq/airgapctl` and Go 1.25.8
- Stub packages exist at `pkg/bundle/`, `pkg/registry/`, `pkg/values/`, `internal/config/`
- `cmd/airgapctl/main.go` exists but references non-existent `internal/cli` package
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

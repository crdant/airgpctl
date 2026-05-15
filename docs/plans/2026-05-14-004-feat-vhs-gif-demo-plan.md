---
title: "feat: VHS GIF demo of airgapctl CLI"
type: feat
status: active
date: 2026-05-14
origin: docs/brainstorms/2026-05-14-airgapctl-requirements.md
---

# feat: VHS GIF demo of airgapctl CLI

## Summary

Create a reproducible terminal demo of `airgapctl` using Charm Bracelet's VHS utility. The demo showcases the three primary commands—`list`, `push`, and `values`—against a realistic mock `.airgap` bundle, pushing images to the public ephemeral registry TTL.sh. Output is a looping GIF embedded in the project README.

---

## Problem Frame

The project README currently says "Usage: Coming soon." A visual demo is the fastest way for SREs and platform engineers to understand what `airgapctl` does and whether it fits their workflow. A static GIF in the README communicates the tool's value without requiring readers to build or install it first.

(see origin: docs/brainstorms/2026-05-14-airgapctl-requirements.md)

---

## Requirements

- R1. Produce a terminal recording (GIF) showing `airgapctl` in action.
- R2. Demo must be reproducible by any contributor running `make demo`.
- R3. Show all three subcommands: `list`, `push`, and `values`.
- R4. GIF must be generated with Charm's VHS utility for consistent, polished output.
- R5. Final GIF must be committed and referenced from the README.

**Origin acceptance examples:**
- AE3 (progress reporting during push) — visible in the GIF.
- AE4 (values generation with custom output path) — visible in the GIF.
- AE5 (idempotent push, skipped images) — visible in the GIF if idempotency is demonstrated.

---

## Scope Boundaries

- **In scope:** VHS tape script, mock fixtures, Makefile targets, README update.
- **Out of scope:** Narrated video, multiple GIFs (one comprehensive GIF is sufficient), CI automation for re-recording the GIF on every release.

### Deferred to Follow-Up Work

- Automated CI check that regenerates the GIF when CLI output changes: future PR when CLI is stable.
- Additional GIFs for specific subcommands (e.g., a focused `push` demo): future PR if requested.

---

## Context & Research

### Relevant Code and Patterns

- `cmd/airgapctl/commands_test.go` — contains `createMockAirgapBundle` and `mockImageLayout` helpers that construct a gzipped tar with `airgap.yaml` and Docker distribution v2 layout. These helpers are the model for demo fixture generation.
- `cmd/airgapctl/commands_test.go` — mock registry via `httptest.NewServer` demonstrates the minimal Docker Registry V2 API surface needed for push (HEAD blobs, POST uploads, PUT manifests).
- `cmd/airgapctl/values.go` — chart directory handling and `values.yaml` parsing logic shows what a minimal chart needs for the `values` subcommand demo.
- `Makefile` — existing build/test targets; `make demo` will be added alongside them.

### Institutional Learnings

- `docs/solutions/best-practices/integration-testing-cli-mock-bundles-registries.md` — documents patterns for mock bundles and registries.
- `docs/solutions/design-patterns/safe-tar-extraction-yaml-parsing-airgap-bundles.md` — bundle format details relevant to fixture creation.

### External References

- [VHS documentation](https://github.com/charmbracelet/vhs) — `.tape` file syntax, terminal theming, output configuration.

---

## Key Technical Decisions

- **Standalone mock bundle generator:** Extract the bundle-creation logic from `commands_test.go` into a small Go program (`demo/generate-bundle.go`) so it can be run independently of the test suite. This avoids relying on `go test` infrastructure for demo setup.
- **TTL.sh for push demo:** Use the public ephemeral registry [TTL.sh](https://ttl.sh) instead of a local mock registry. TTL.sh was built by Replicated for exactly this kind of short-lived, throwaway image testing. Images auto-delete after 1 hour, no credentials are required, and there is zero local infrastructure to manage. This eliminates mock registry code, background process orchestration, and port collision risks entirely.
- **Single comprehensive GIF:** One `.tape` file demonstrates `list`, `push`, and `values` in sequence rather than splitting into multiple GIFs. The narrative flow (inspect → push → generate values) matches the SRE workflow.
- **VHS + ttyd via Homebrew:** macOS is the primary development environment (darwin/arm64 detected). Both `vhs` and `ttyd` are available via `brew install`. The Makefile will check presence and print install instructions when missing.

---

## Open Questions

### Resolved During Planning

- **Q:** Should the demo include the `push` command given it needs a registry?  
  **A:** Yes — push is a primary differentiator of the tool. We will use TTL.sh (Replicated's public ephemeral registry) for the demo, which requires no local setup.
- **Q:** Should the bundle contain a multi-arch image?  
  **A:** Yes — include one multi-arch image (`nginx:latest`) and two single-arch images (`redis:7`, `postgres:15`) to showcase the `list` output variety and partial chart-aware matching in `values`.
- **Q:** What terminal theme and font?  
  **A:** Use the "Rose Pine Moon" theme with the "Inconsolata" font at 1200x800 for crisp README rendering.

### Deferred to Implementation

- **Q:** Exact VHS `Sleep` durations between commands.  
  **A:** Tuned during implementation after seeing actual command execution speed on the target machine.
- **Q:** Whether to include a `--verbose` flag demonstration.  
  **A:** Decided during tape scripting — include only if it improves clarity without clutter.

---

## Output Structure

```
demo/
├── airgapctl-demo.tape      # VHS script
├── generate-bundle.go       # Standalone bundle generator
├── fixtures/
│   ├── chart/
│   │   ├── Chart.yaml       # Minimal chart metadata
│   │   └── values.yaml      # Chart values with image references
│   └── .gitkeep             # Ensure fixtures/ is tracked even when empty
├── run-demo.sh              # Orchestration wrapper script
└── airgapctl-demo.gif       # Generated output (tracked in git LFS or committed)
```

---

## Implementation Units

### U1. Install VHS tooling and scaffold demo directory

**Goal:** Ensure VHS and its dependency `ttyd` are discoverable on the development machine, and create the `demo/` directory structure.

**Requirements:** R4

**Dependencies:** None

**Files:**
- Create: `demo/fixtures/.gitkeep`
- Modify: `Makefile`
- Modify: `.gitignore`

**Approach:**
- Add a `demo-deps` Makefile target that checks for `vhs` and `ttyd` via `command -v`, printing `brew install` instructions when absent.
- Add a `demo-clean` target that removes generated artifacts (`demo/*.gif`, `demo/fixtures/*.airgap`, temp registry binary).
- Update `.gitignore` to ignore `demo/fixtures/*.airgap` and any temporary binaries built under `demo/`.

**Patterns to follow:**
- Existing Makefile target style (`build`, `test`, `e2e`).

**Test scenarios:**
- Test expectation: none -- this is pure tooling/config scaffolding.

**Verification:**
- `make demo-deps` runs and prints actionable instructions when `vhs` is missing.
- `make demo-clean` removes `demo/*.gif`, `demo/fixtures/*.airgap`, and temp binaries without error.

---

### U2. Create demo fixtures (mock bundle and Helm chart)

**Goal:** Produce a realistic `.airgap` bundle and a minimal Helm chart for use in the VHS demo.

**Requirements:** R2, R3

**Dependencies:** U1

**Files:**
- Create: `demo/generate-bundle.go`
- Create: `demo/fixtures/chart/Chart.yaml`
- Create: `demo/fixtures/chart/values.yaml`
- Modify: `Makefile`

**Approach:**
- `generate-bundle.go` is a standalone Go program (not a test) that writes `demo/fixtures/bundle.airgap`. It reuses the tar/gzip construction pattern from `cmd/airgapctl/commands_test.go` but is self-contained and runnable with `go run`.
- The bundle contains:
  - `airgap.yaml` with `savedImages`: `nginx:latest` (multi-arch), `redis:7` (single-arch), `postgres:15` (single-arch).
  - Docker distribution v2 layout with manifest lists for `nginx:latest` and single-arch manifests for `redis:7` and `postgres:15`.
- `demo/fixtures/chart/Chart.yaml` contains minimal Helm metadata (`apiVersion: v2`, `name: demo-app`, `version: 0.1.0`).
- `demo/fixtures/chart/values.yaml` references `nginx` and `redis` images (but not `postgres`) so the `values` subcommand demo shows partial chart-aware matching.

**Technical design:**
- The manifest list JSON follows the `application/vnd.docker.distribution.manifest.list.v2+json` schema with two platform entries (`linux/amd64`, `linux/arm64`), matching the pattern in `TestListCommand_MultiArch`.
- Child manifests reuse the same single-arch manifest JSON structure as `makeSingleArchManifest`.
- **Critical:** The bundle must also include the child manifest blobs at `images/docker/registry/v2/blobs/sha256/<prefix>/<digest>/data` so the `push` subcommand can read them during `pushMultiArch`. The `TestListCommand_MultiArch` test only writes the list manifest, but the real pusher iterates over child manifests and reads each blob via `walker.ReadBlob(pm.Digest)`.

**Patterns to follow:**
- `cmd/airgapctl/commands_test.go` — tar/gzip construction, manifest JSON shapes, directory layout paths.

**Test scenarios:**
- Happy path: Running `go run demo/generate-bundle.go` produces `demo/fixtures/bundle.airgap` that `airgapctl list` can parse and display.
- Edge case: Re-running the generator overwrites the previous fixture cleanly.

**Verification:**
- `go run demo/generate-bundle.go` succeeds.
- `./bin/airgapctl list --bundle demo/fixtures/bundle.airgap` prints three images with correct types (`multi-arch` for nginx, `single-arch` for redis and postgres).

---

### U3. Verify TTL.sh push compatibility

**Goal:** Confirm the `airgapctl push` subcommand works against the public ephemeral registry TTL.sh before recording the demo.

**Requirements:** R3

**Dependencies:** U2

**Files:**
- None (manual verification step)

**Approach:**
- TTL.sh is a public, ephemeral registry built by Replicated that accepts anonymous pushes and auto-deletes images after 1 hour. No local infrastructure is required.
- Run a one-off manual test: `./bin/airgapctl push --bundle demo/fixtures/bundle.airgap --registry ttl.sh --namespace airgapctl-demo-$(date +%s) --username unused --password unused --tls-skip-verify`
- The `--username` and `--password` flags are required by `airgapctl`'s `PreRunE` validation (see `push.go:31-45`), but TTL.sh ignores them. Any non-empty values satisfy the CLI.
- The `--tls-skip-verify` flag is not needed for TTL.sh (it has a valid TLS cert), but including it keeps the command resilient.
- This verification step ensures the push flow, progress reporting, and per-image status output are functional before the VHS recording begins.

**Test scenarios:**
- Happy path: Push completes with "3 pushed, 0 skipped, 0 failed".
- Error path: If TTL.sh is unreachable, the verification fails fast and the user is warned before the VHS recording begins.

**Verification:**
- Manual test run succeeds against `ttl.sh` with the demo bundle.
- Output shows progress bars and the final summary line.

---

### U4. Write VHS tape script

**Goal:** Script the terminal recording that demonstrates `airgapctl` in a polished, visually consistent way.

**Requirements:** R1, R3, R4

**Dependencies:** U3

**Files:**
- Create: `demo/airgapctl-demo.tape`

**Approach:**
- Terminal configuration: `Set FontSize 18`, `Set FontFamily "Inconsolata"`, `Set Width 1200`, `Set Height 800`, `Set Theme "Rose Pine Moon"`, `Set TypingSpeed 50ms`.
- Pre-demo setup: `Hide` the shell prompt, type a comment block that clears the screen and builds the binary, then `Show`.
- Narrative sequence:
  1. Build the binary: `make build`
  2. Show help: `./bin/airgapctl --help`
  3. List images: `./bin/airgapctl list --bundle demo/fixtures/bundle.airgap`
  4. Push images: `./bin/airgapctl push --bundle demo/fixtures/bundle.airgap --registry ttl.sh --namespace airgapctl-demo --username unused --password unused`
  5. Generate values: `./bin/airgapctl values --bundle demo/fixtures/bundle.airgap --chart demo/fixtures/chart --registry myregistry.example.com --namespace airgap --output demo/values-airgap.yaml`
  6. Show generated values file: `cat demo/values-airgap.yaml`
- Each command is followed by a `Sleep` long enough for the output to be readable (typically 1–2 seconds after output stabilizes).
- The `Output` directive points to `demo/airgapctl-demo.gif`.

**Patterns to follow:**
- VHS tape conventions from the Charm Bracelet project examples.

**Test scenarios:**
- Test expectation: none -- this is a VHS script, not code. Correctness is verified by running VHS.

**Verification:**
- `vhs demo/airgapctl-demo.tape` runs without error when fixtures exist and network access to TTL.sh is available.
- The resulting `demo/airgapctl-demo.gif` plays in a browser and clearly shows all three subcommands.

---

### U5. Generate GIF, add Makefile target, and update README

**Goal:** Orchestrate the full demo pipeline and surface the GIF in the project README.

**Requirements:** R2, R5

**Dependencies:** U4

**Files:**
- Create: `demo/run-demo.sh`
- Modify: `Makefile`
- Modify: `README.md`
- Create: `demo/airgapctl-demo.gif` (generated artifact)

**Approach:**
- Create `demo/run-demo.sh` — a shell wrapper that orchestrates the full pipeline:
  1. `make build`
  2. `go run demo/generate-bundle.go`
  3. `vhs demo/airgapctl-demo.tape`
- Add `make demo` target that invokes `demo/run-demo.sh`.
- No local registry setup is needed because the tape pushes to the public ephemeral registry TTL.sh.
- The README is updated to:
  - Embed the GIF near the top (below the one-line description).
  - Replace "Usage: Coming soon." with a concise usage section showing the three primary commands with their most common flags.
  - Reference `make demo` for contributors who want to re-record.

**Patterns to follow:**
- Existing README structure (title, description, development, usage).

**Test scenarios:**
- Test expectation: none -- this is documentation and orchestration.

**Verification:**
- `make demo` completes end-to-end and produces `demo/airgapctl-demo.gif`.
- README renders correctly on GitHub with the GIF visible.
- Re-running `make demo` is idempotent (overwrites GIF, cleans up temp processes).

---

## System-Wide Impact

- **New directory:** `demo/` is a documentation/demo asset directory, not production code. It does not affect the binary build or test suite.
- **Makefile additions:** `demo`, `demo-deps`, `demo-clean`, `demo-registry` targets are additive and do not change existing `build`, `test`, or `e2e` behavior.
- **Git tracking:** The generated GIF is a binary asset. It should be committed to git (small enough for a 15-second GIF, typically < 1 MB) or added to git LFS if it grows larger. The `.gitignore` update excludes transient demo artifacts (`*.airgap`, temp binaries) from the working tree.
- **Unchanged invariants:** No CLI flags, package APIs, or test logic are modified by this plan.

---

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| VHS or ttyd not installed on contributor machine | `make demo-deps` prints exact `brew install` commands; VHS is only needed for GIF generation, not for normal development |
| TTL.sh unreachable during recording | The VHS tape pushes to a public registry; if the network is down, the recording will fail. Run `make demo` only when online, or retry. |
| GIF file size grows large | Keep the tape short (under 30 seconds) and terminal dimensions moderate; fall back to git LFS if needed |
| VHS rendering differs across macOS versions | Pin VHS theme and font explicitly in the tape file; avoid OS-specific color profiles |

---

## Documentation / Operational Notes

- README update is the primary documentation deliverable.
- `make demo` should be mentioned in a "Demo" or "Recording" subsection of README for contributors.
- If the CLI output changes (new flags, changed progress format), the tape script may need re-recording. This is expected and documented as "Deferred to Follow-Up Work" above.

---

## Sources & References

- **Origin document:** [docs/brainstorms/2026-05-14-airgapctl-requirements.md](docs/brainstorms/2026-05-14-airgapctl-requirements.md)
- **Related code:** `cmd/airgapctl/commands_test.go` — mock bundle and registry patterns.
- **Related code:** `cmd/airgapctl/list.go`, `cmd/airgapctl/push.go`, `cmd/airgapctl/values.go` — subcommand implementations.
- **External docs:** [Charm VHS](https://github.com/charmbracelet/vhs) — `.tape` syntax and theming.

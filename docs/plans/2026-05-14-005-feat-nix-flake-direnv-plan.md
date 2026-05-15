---
title: "feat: Nix flake with direnv for reproducible dev environment"
type: feat
status: active
date: 2026-05-14
---

# feat: Nix flake with direnv for reproducible dev environment

## Summary

Add a `flake.nix` that exposes both the `airgapctl` package (via `buildGoModule`) and a `devShell` with Go, VHS, and all tools needed to build, test, lint, and record demos. A `.envrc` activates the shell automatically via `direnv`, so any contributor with Nix installed gets the exact same toolchain on entry.

---

## Requirements

- R1. `nix develop` (or `direnv` auto-activation) drops the user into a shell with Go, `make`, `git`, `golangci-lint`, `vhs`, `ttyd`, and `ffmpeg`.
- R2. `.envrc` loads the flake with `use flake` so the environment is automatic on `cd` into the repo.
- R3. The flake is pure-Nix — no Homebrew, no manual `go install`, no external package managers required.
- R4. README documents the one-liner setup for Nix + direnv users.
- R5. `nix build` (or `nix run`) produces the `airgapctl` binary from the flake's `packages.default` output.

---

## Scope Boundaries

- **In scope:** `flake.nix`, `flake.lock`, `.envrc`, `.gitignore` update, README "Getting Started" addition.
- **Out of scope:** CI integration with Nix, Docker image derivation.

---

## Key Technical Decisions

- **Pinned nixpkgs-unstable:** Use `github:NixOS/nixpkgs/nixpkgs-unstable` as the flake input and commit the generated `flake.lock`. Unstable is required because the project uses Go 1.25.8, which is not available in NixOS 24.11 (the latest stable at plan time). Pinning ensures every contributor gets bit-identical packages.
- **Package via `buildGoModule`:** The `packages.default` output uses `pkgs.buildGoModule` with `src = self;` and `vendorHash = null;` (the project uses Go modules but may vendor — the implementer sets the correct hash after `nix build` fails the first time). This is the standard Nix pattern for packaging Go CLI tools.
- **Go in devShell:** The devShell simply includes `pkgs.go` (which resolves to Go 1.25.x on the pinned unstable channel) in `mkShell` `buildInputs`. No dynamic parsing of `go.mod` at evaluation time — the Go version tracks nixpkgs and is verified manually.
- **direnv watch:** Add `watch_file go.mod` and `watch_file flake.nix` to `.envrc` so the shell reloads when dependencies or flake inputs change.

---

## Implementation Units

### U1. Create flake.nix and generate flake.lock

**Goal:** Define the Nix flake with a `packages.default` derivation for `airgapctl` and a `devShells.default` containing all required tools.

**Requirements:** R1, R3, R5

**Dependencies:** None

**Files:**
- Create: `flake.nix`
- Create: `flake.lock`

**Approach:**
- Use `inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";`.
- Structure `outputs` with per-system support using `nixpkgs.lib.genAttrs [ "aarch64-darwin" "x86_64-linux" ]` so the flake evaluates on both Darwin (primary dev host) and Linux (CI / contributors).
- `packages.default = pkgs.buildGoModule { pname = "airgapctl"; version = "0.1.0"; src = self; vendorHash = "<to-be-set-after-first-build>"; };`
- `devShells.default = pkgs.mkShell { inputsFrom = [ self.packages.${system}.default ]; buildInputs = [ pkgs.gnumake pkgs.git pkgs.golangci-lint pkgs.vhs pkgs.ttyd pkgs.ffmpeg ]; };`
- Run `nix flake update` to generate `flake.lock` and commit it.

**Patterns to follow:**
- Standard Nix flake structure with `genAttrs` for multi-system support.

**Test scenarios:**
- Manual verification: `nix develop --command go version` prints Go 1.25.x.
- Manual verification: `nix develop --command vhs --version` prints a version number.
- Manual verification: `nix develop --command ttyd --version` prints a version number.
- Manual verification: `nix develop --command make build` succeeds from a clean shell.
- Manual verification: `nix build` produces `./result/bin/airgapctl` that runs and prints `--help`.

**Verification:**
- All five manual verification commands above succeed.

---

### U2. Create .envrc and update .gitignore

**Goal:** Enable automatic flake activation via `direnv` and ignore direnv artifacts.

**Requirements:** R2

**Dependencies:** U1

**Files:**
- Create: `.envrc`
- Modify: `.gitignore`

**Approach:**
- `.envrc` contains:
  ```bash
  use flake
  watch_file go.mod
  watch_file flake.nix
  ```
- `.gitignore` adds `.direnv/` to prevent caching noise in git status.

**Verification:**
- `direnv allow` succeeds and `PATH` inside the project directory includes `vhs`, `go`, `make`, and `airgapctl` from the Nix store.

---

### U3. Update README with Nix setup instructions

**Goal:** Document the Nix path and reconcile the existing manual prerequisites with the project's actual Go version.

**Requirements:** R4

**Dependencies:** U2

**Files:**
- Modify: `README.md`

**Approach:**
- Update the existing "Prerequisites" section from "Go 1.21 or later" to "Go 1.25 or later" (matching `go.mod`).
- Add a "Getting Started (with Nix)" subsection:
  1. "Install Nix with flakes enabled and `direnv`."
  2. "Run `direnv allow` in the repo root — all tools (Go, VHS, ttyd, ffmpeg, golangci-lint) are provided automatically."
  3. "Build: `make build` or `nix build`."
- Keep the existing manual prerequisites for non-Nix users, updated to Go 1.25+.

**Verification:**
- README renders correctly on GitHub.
- The Go version stated in manual prerequisites matches the version in `go.mod`.
- Nix instructions are discoverable at a glance.

---

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| `vhs` or `ttyd` not available in pinned nixpkgs-unstable | Verify with `nix search` before committing `flake.lock`; if absent, pin a more recent unstable rev or add an overlay |
| `go` in pinned nixpkgs-unstable is not 1.25.x | Acceptable if it's 1.24+; the project likely compiles. If incompatible, bump the nixpkgs pin to a newer unstable revision |
| Flake evaluation slow on first run | Acceptable one-time cost; `direnv` caches subsequent activations |
| `vendorHash` mismatch on first `nix build` | Expected — Nix will print the correct hash; update `flake.nix` with the computed hash and rebuild |

---

## Sources & References

- **Existing code:** `go.mod` — Go version constraint (1.25.8).
- **Existing code:** `Makefile` — lists tools consumed by the project (`go`, `golangci-lint`).
- **External docs:** [Nix flakes](https://nixos.org/manual/nix/stable/command-ref/new-cli/nix3-flake.html), [direnv Nix integration](https://direnv.net/man/direnv-stdlib.1.html#use-flake).

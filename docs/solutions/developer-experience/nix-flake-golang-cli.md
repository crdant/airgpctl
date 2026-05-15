---
title: "Nix flake with buildGoModule for Go CLI dev environment"
date: 2026-05-14
type: knowledge
category: developer-experience
tags: [nix, flake, golang, buildgomodule, direnv, devshell]
component: build-system
problem_type: tooling-decisions
---

# Nix flake with buildGoModule for Go CLI dev environment

## Context

The project needed a reproducible development environment for a Go CLI (`airgapctl`) that could build the binary, provide all dev tools (linter, VHS for demos, ttyd, ffmpeg), and work across Darwin and Linux without relying on Homebrew or manual package installation.

## Guidance

Use a pure-Nix flake with `buildGoModule` for the package derivation and `mkShell` for the dev shell. Pin `nixpkgs-unstable` to get recent Go versions (1.25+ was required; nixpkgs-unstable provided 1.26.x at the time).

### Minimal flake structure

```nix
{
  description = "airgapctl - CLI for loading Replicated .airgap bundles";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "aarch64-darwin" "x86_64-darwin" "aarch64-linux" "x86_64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.buildGoModule {
            pname = "airgapctl";
            version = "0.1.0";
            src = self;
            vendorHash = null;   # only if no vendor/ dir
          };
        }
      );

      devShells = forAllSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShell {
            inputsFrom = [ self.packages.${system}.default ];
            buildInputs = [
              pkgs.gnumake
              pkgs.git
              pkgs.golangci-lint
              pkgs.vhs
              pkgs.ttyd
              pkgs.ffmpeg
            ];
          };
        }
      );
    };
}
```

### Key decisions

- **`vendorHash = null`**: Use when the project has no `vendor/` directory and relies purely on Go modules fetched at build time. Nix will fetch modules into its own store automatically.
- **`inputsFrom`**: Propagates `buildInputs` and `nativeBuildInputs` from the package derivation into the dev shell, so the same Go compiler is used for both `nix build` and `nix develop`.
- **`genAttrs` over `flake-utils`**: Keeps the flake dependency-free. `nixpkgs.lib.genAttrs [ ... ]` is sufficient for multi-system support.

### .gitignore additions

Always ignore Nix build result symlinks:

```gitignore
# Nix build outputs
result
result-*
```

## Why This Matters

Without a flake, every contributor installs tools differently (Homebrew, apt, go install, etc.). Version drift causes "works on my machine" build failures and CI surprises. A flake guarantees identical toolchains and makes onboarding a single `direnv allow` away.

## When to Apply

- Any Go project where reproducible builds and dev environments matter
- When you need tools that are easy to pin in nixpkgs (e.g., `vhs`, `ttyd`, `golangci-lint`) but painful to install consistently across macOS and Linux
- Before adding `.envrc` so that `direnv` can auto-activate the shell on `cd`

## Examples

### Verification script

A shell script can assert the flake meets its contract:

```bash
#!/usr/bin/env bash
set -euo pipefail
trap 'rm -f result' EXIT

nix develop --command go version
nix develop --command vhs --version
nix develop --command ttyd --version
nix develop --command make build
nix build
./result/bin/airgapctl --help
```

### Common pitfall: missing main package

`buildGoModule` requires a `package main` in the module to produce a binary. If `cmd/airgapctl/main.go` does not exist, `nix build` succeeds but produces no `./result/bin/airgapctl`. Always verify the binary exists after the first successful build.

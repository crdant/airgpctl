# airgapctl

A Go CLI tool for loading Replicated `.airgap` bundles into a private registry and generating Helm values files.

## Development

### Getting Started (with Nix)

Install Nix with flakes enabled and `direnv`.

Run `direnv allow` in the repo root — all tools (Go, VHS, ttyd, ffmpeg, golangci-lint) are provided automatically.

Build:

```bash
make build
# or
nix build
```

### Prerequisites (manual)

- Go 1.25 or later
- Make

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Linting

```bash
make lint
```

## Usage

Coming soon.

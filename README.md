# airgapctl

A Go CLI tool for loading Replicated `.airgap` bundles into a private registry and generating Helm values files.

![Demo](demo/airgapctl-demo.gif)

## Development

### Prerequisites

- Go 1.21 or later
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

### Demo

To regenerate the demo GIF (requires [VHS](https://github.com/charmbracelet/vhs) and `ttyd`):

```bash
make demo
```

## Usage

`airgapctl` has three primary commands: `list`, `push`, and `values`.

### `list` — inspect bundle contents

```bash
airgapctl list --bundle <path-to-bundle.airgap>
```

### `push` — load images into a registry

```bash
airgapctl push \
  --bundle <path-to-bundle.airgap> \
  --registry <registry-host> \
  --namespace <target-namespace> \
  --username <user> \
  --password <pass>
```

### `values` — generate Helm values with remapped image paths

```bash
airgapctl values \
  --bundle <path-to-bundle.airgap> \
  --chart <path-to-helm-chart> \
  --registry <registry-host> \
  --namespace <target-namespace> \
  --output <output-values.yaml>
```

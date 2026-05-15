#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${PROJECT_ROOT}"

echo "=== Building airgapctl ==="
make build

echo "=== Generating demo fixtures ==="
go run demo/generate-bundle.go

echo "=== Recording VHS demo ==="
vhs demo/airgapctl-demo.tape

echo "=== Demo GIF generated: demo/airgapctl-demo.gif ==="

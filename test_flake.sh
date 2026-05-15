#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Test 1: nix develop --command go version ==="
go_version=$(nix develop --command go version 2>/dev/null | head -1)
echo "$go_version"
if ! echo "$go_version" | grep -qE "go1\.(2[4-9]|[3-9][0-9])"; then
    echo "FAIL: Expected Go 1.24+ in output (got: $go_version)"
    exit 1
fi
echo "PASS"

echo ""
echo "=== Test 2: nix develop --command vhs --version ==="
if ! nix develop --command vhs --version >/dev/null 2>&1; then
    echo "FAIL: vhs --version failed"
    exit 1
fi
echo "PASS"

echo ""
echo "=== Test 3: nix develop --command ttyd --version ==="
if ! nix develop --command ttyd --version >/dev/null 2>&1; then
    echo "FAIL: ttyd --version failed"
    exit 1
fi
echo "PASS"

echo ""
echo "=== Test 4: nix develop --command make build ==="
if ! nix develop --command make build >/dev/null 2>&1; then
    echo "FAIL: make build failed"
    exit 1
fi
echo "PASS"

echo ""
echo "=== Test 5: nix build produces ./result/bin/airgapctl ==="
rm -f result
if ! nix build >/dev/null 2>&1; then
    echo "FAIL: nix build failed"
    exit 1
fi
if [ ! -x "./result/bin/airgapctl" ]; then
    echo "FAIL: ./result/bin/airgapctl not found or not executable"
    rm -f result
    exit 1
fi
help_output=$(./result/bin/airgapctl --help 2>&1 || true)
echo "$help_output"
if ! echo "$help_output" | grep -qi "usage\|help"; then
    echo "FAIL: Expected usage/help output from --help"
    rm -f result
    exit 1
fi
rm -f result
echo "PASS"

echo ""
echo "=== All tests passed ==="

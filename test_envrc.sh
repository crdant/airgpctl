#!/usr/bin/env bash
# test_envrc.sh - Test script for U2: .envrc and .gitignore verification
# Follows RED-GREEN-REFACTOR workflow

set -euo pipefail

ERRORS=0

echo "=== Testing U2: .envrc and .gitignore ==="

# Test 1: .envrc exists and contains required content
echo "--- Test 1: .envrc content ---"
if [ ! -f .envrc ]; then
    echo "FAIL: .envrc does not exist"
    ERRORS=$((ERRORS + 1))
else
    echo "PASS: .envrc exists"
    
    if grep -q "use flake" .envrc; then
        echo "PASS: .envrc contains 'use flake'"
    else
        echo "FAIL: .envrc missing 'use flake'"
        ERRORS=$((ERRORS + 1))
    fi
    
    if grep -q "watch_file go.mod" .envrc; then
        echo "PASS: .envrc contains 'watch_file go.mod'"
    else
        echo "FAIL: .envrc missing 'watch_file go.mod'"
        ERRORS=$((ERRORS + 1))
    fi
    
    if grep -q "watch_file flake.nix" .envrc; then
        echo "PASS: .envrc contains 'watch_file flake.nix'"
    else
        echo "FAIL: .envrc missing 'watch_file flake.nix'"
        ERRORS=$((ERRORS + 1))
    fi
fi

# Test 2: .direnv/ is in .gitignore
echo "--- Test 2: .gitignore contains .direnv/ ---"
if git check-ignore -q .direnv/ 2>/dev/null; then
    echo "PASS: .direnv/ is ignored by git"
else
    echo "FAIL: .direnv/ is not ignored by git"
    ERRORS=$((ERRORS + 1))
fi

# Test 3: direnv exec . go version works
echo "--- Test 3: direnv exec . go version ---"
if direnv exec . go version > /dev/null 2>&1; then
    GO_VERSION=$(direnv exec . go version 2>/dev/null)
    echo "PASS: direnv exec . go version works: $GO_VERSION"
else
    echo "FAIL: direnv exec . go version failed"
    ERRORS=$((ERRORS + 1))
fi

# Summary
echo ""
echo "=== Summary ==="
if [ $ERRORS -eq 0 ]; then
    echo "All tests PASSED"
    exit 0
else
    echo "$ERRORS test(s) FAILED"
    exit 1
fi

---
title: "Cobra CLI scaffolding with test-first flag validation and credential fallback"
date: 2026-05-14
category: best-practices
module: cli
problem_type: best_practice
component: development_workflow
severity: low
applies_when:
  - "Implementing a new Go CLI with subcommands using Cobra"
  - "Adding flag validation and credential resolution to CLI commands"
  - "Writing table-driven tests for CLI command parsing and pre-run hooks"
tags:
  - cobra
  - cli
  - go
  - test-first
  - docker-config
  - credentials
---

# Cobra CLI scaffolding with test-first flag validation and credential fallback

## Context

When building `airgapctl` as a multi-subcommand Go CLI, we needed a clean Cobra-based command structure with persistent flags, per-subcommand validation, and credential resolution (flags → env vars → Docker config). The scaffolding needed to be testable from the start so downstream units (U2–U6) could build on a verified CLI skeleton.

## Guidance

### 1. Keep command handlers thin; delegate to core packages

Command `RunE` functions should parse flags and call packages in `pkg/` and `internal/`. Avoid business logic in `cmd/`.

### 2. Use `PersistentPreRunE` for shared validation, `PreRunE` for subcommand-specific checks

Cobra executes `PersistentPreRunE` before `PreRunE`. This ordering matters: if the shared validation (e.g., bundle path existence) fails first, subcommand-specific errors (e.g., missing `--registry`) may be hidden. In tests, provide a valid bundle path when testing subcommand-specific validation so `PreRunE` errors surface.

### 3. Test flag parsing with `ExecuteC`, not `Execute`

`ExecuteC` returns the executed `*cobra.Command` and the error, letting you assert on both without relying on side effects like `os.Exit`:

```go
func executeCommand(cmd *cobra.Command, args ...string) (string, string, *cobra.Command, error) {
    bufOut := new(bytes.Buffer)
    bufErr := new(bytes.Buffer)
    cmd.SetOut(bufOut)
    cmd.SetErr(bufErr)
    cmd.SetArgs(args)
    c, err := cmd.ExecuteC()
    return bufOut.String(), bufErr.String(), c, err
}
```

### 4. Resolve bundle path with flag-precedence-over-positional logic

When the bundle path can come from either `--bundle` or a positional argument, make `--bundle` win explicitly:

```go
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
    if globalOpts.bundle == "" && len(args) > 0 {
        globalOpts.bundle = args[0]
    }
    // ... validate file exists and is not a directory
}
```

### 5. Validate that the bundle path is a file, not just "exists"

`os.Stat` returns nil for directories. Explicitly reject directories to avoid confusing downstream logic that expects a tar archive:

```go
fi, err := os.Stat(abs)
if err != nil {
    return fmt.Errorf("bundle file not found: %w", err)
}
if fi.IsDir() {
    return fmt.Errorf("bundle path is a directory, expected a file: %s", abs)
}
```

### 6. Implement credential fallback chain fully

The chain should be: CLI flags → env vars → Docker config. Each layer should only apply if the layer above provided nothing.

For Docker config, support the `DOCKER_CONFIG` environment variable for custom config directories. Decode the `auth` base64 string (`username:password`) and strip scheme/port prefixes when matching the registry key:

```go
func loadDockerConfig() error {
    var configPath string
    if envDir := os.Getenv("DOCKER_CONFIG"); envDir != "" {
        configPath = filepath.Join(envDir, "config.json")
    } else {
        home, _ := os.UserHomeDir()
        configPath = filepath.Join(home, ".docker", "config.json")
    }
    // ... parse, match registry, decode base64 auth
}
```

### 7. Guard against .gitignore matching directory names

A `.gitignore` entry like `airgapctl` (the binary name) also matches `cmd/airgapctl/`. Use `/airgapctl` to anchor the pattern to the repository root:

```
# Build artifacts
bin/
dist/
/airgapctl
```

## Why This Matters

A well-tested CLI skeleton prevents regressions when later units add real behavior (bundle extraction, registry push, values generation). Without tests for flag parsing and validation, small refactors in command wiring can break UX silently. Without a real credential fallback implementation, the "stub" becomes a hidden bug that only surfaces in production air-gapped environments.

## When to Apply

- Starting a new Go CLI with Cobra
- Adding subcommands that require pre-run validation
- Implementing credential resolution with multiple fallback layers
- Creating test fixtures for CLI commands that depend on file-system state

## Examples

### Before (credential fallback stub)

```go
if pushOpts.username == "" && pushOpts.token == "" {
    _ = loadDockerConfig() // silently ignored, no actual credential use
}
```

### After (functional fallback)

```go
if pushOpts.username == "" && pushOpts.token == "" {
    _ = loadDockerConfig() // populates pushOpts.username/password if found
}
```

### Before (.gitignore)

```
airgapctl
```

### After (.gitignore)

```
/airgapctl
```

## Related

- [Cobra documentation](https://github.com/spf13/cobra)
- Plan: [docs/plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md](../plans/2026-05-14-001-feat-cobra-cli-subcommands-plan.md)

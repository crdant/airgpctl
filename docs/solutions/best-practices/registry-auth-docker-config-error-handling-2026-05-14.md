---
title: "Registry auth and docker-config error handling: surfacing failures and parsing edge cases"
date: 2026-05-14
category: best-practices
module: pkg/registry
problem_type: best_practice
component: service_object
severity: medium
applies_when:
  - "Implementing Docker registry authentication from config files"
  - "Parsing image references with digest components"
  - "Normalizing registry URLs and matching against docker-config auths"
  - "Writing defensive constructors that avoid mutating caller-provided structs"
tags:
  - docker-config
  - registry-auth
  - error-handling
  - image-parsing
  - config-immutability
  - go
---

# Registry auth and docker-config error handling: surfacing failures and parsing edge cases

## Context

During U1 implementation ("Wire bearer-token auth and surface docker-config errors"), code review surfaced five robustness gaps where errors were silently swallowed, edge cases in image reference parsing were unhandled, and a constructor mutated its input parameter. These issues all share a common theme: defensive coding against real-world inputs that are almost-right but subtly malformed. The fixes apply to any Go code that reads Docker config, parses OCI image references, or normalizes registry URLs.

## Guidance

### 1. Wrap filesystem and parse errors with context

Never return raw `os.ReadFile` or `json.Unmarshal` errors directly from user-facing config loading functions. Include the file path in the error so operators immediately know which file failed and why.

### 2. Try scheme-prefixed registry keys when matching docker-config auths

Docker config `auths` keys may include `https://` or `http://` prefixes, but CLI flags often omit the scheme. When the user provides a bare registry hostname, also try `https://` + host and `http://` + host as lookup keys before concluding no credentials exist.

### 3. Validate base64-decoded auth fields explicitly

Docker config stores credentials as base64-encoded `username:password`. After decoding, check that the result actually contains a `:` separator. Return an explicit error if it is missing rather than silently ignoring the malformed entry and continuing unauthenticated.

### 4. Strip digest components (`@sha256:...`) before tag stripping

Image references may use digest syntax: `repo@sha256:abc`. If you strip the tag (`:`) first, the digest portion (`sha256:abc`) looks like a tag and gets mangled into a bogus path. Always strip the digest separator `@` first, then strip the tag separator `:`.

### 5. Do not mutate caller-provided structs in constructors

If a constructor needs to normalize a field (e.g., prepend `https://` to a bare registry URL), copy the value into a local variable or create a new struct. Mutating the passed parameter creates hidden side effects that break tests and surprise callers.

## Why This Matters

Silent failures in authentication paths are especially dangerous in air-gapped deployments because the tool appears to initialize correctly but pushes fail later with cryptic 401/403 errors. Digest references are common in Replicated bundles; mangling them produces incorrect destination repository paths that point to non-existent images. Mutating caller structs creates subtle state bugs that are hard to reproduce in isolation — a test may pass when run alone but fail when ordered after another test that reuses the same config struct.

## When to Apply

- Loading docker-config `auths` for registry push or pull operations
- Parsing image references that may contain tags, digests, or both
- Matching registry hostnames against configuration dictionaries with inconsistent key formatting
- Writing package-level constructors that normalize, default, or validate input values

## Examples

### Before: raw errors without context

```go
func loadDockerConfig() error {
    // ... resolve configPath ...
    data, err := os.ReadFile(configPath)
    if err != nil {
        return err  // "no such file or directory" — which file?
    }
    var cfg dockerConfig
    if err := json.Unmarshal(data, &cfg); err != nil {
        return err  // "invalid character ..." — in which file?
    }
    // ...
}
```

### After: wrapped errors with file path

```go
func loadDockerConfig() error {
    // ... resolve configPath ...
    data, err := os.ReadFile(configPath)
    if err != nil {
        return fmt.Errorf("loading docker config %s: %w", configPath, err)
    }
    var cfg dockerConfig
    if err := json.Unmarshal(data, &cfg); err != nil {
        return fmt.Errorf("parsing docker config %s: %w", configPath, err)
    }
    // ...
}
```

### Before: missing scheme-prefixed lookup keys

```go
// Registry passed as "myregistry.example.com"
// Docker config key is "https://myregistry.example.com"
candidates := []string{registry}  // never matches
```

### After: scheme-prefixed candidates

```go
var candidates []string
candidates = append(candidates, registry)
if !strings.HasPrefix(registry, "https://") && !strings.HasPrefix(registry, "http://") {
    candidates = append(candidates, "https://"+registry, "http://"+registry)
}
// ... try each candidate against dockerConfig.auths
```

### Before: silently ignoring malformed auth

```go
parts := strings.SplitN(string(decoded), ":", 2)
if len(parts) == 2 {
    pushOpts.username = parts[0]
    pushOpts.password = parts[1]
    return nil
}
// falls through silently — no credentials set, push will later fail with 401
```

### After: explicit error on malformed auth

```go
parts := strings.SplitN(string(decoded), ":", 2)
if len(parts) != 2 {
    return fmt.Errorf("invalid auth format for %s: expected username:password", key)
}
pushOpts.username = parts[0]
pushOpts.password = parts[1]
return nil
```

### Before: tag stripping before digest stripping

```go
func imagePath(ref string) string {
    if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
        ref = ref[:idx]  // "repo@sha256:abc" → "repo@sha256" (wrong!)
    }
    // ...
}
```

### After: digest stripping first

```go
func imagePath(ref string) string {
    if idx := strings.LastIndex(ref, "@"); idx != -1 {
        ref = ref[:idx]  // "repo@sha256:abc" → "repo"
    }
    if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
        ref = ref[:idx]  // "repo:tag" → "repo"
    }
    // ...
}
```

### Before: constructor mutates caller struct

```go
func NewPusher(cfg Config) *Pusher {
    if !strings.HasPrefix(cfg.Registry, "http://") && !strings.HasPrefix(cfg.Registry, "https://") {
        cfg.Registry = "https://" + cfg.Registry  // mutates caller's cfg!
    }
    return &Pusher{cfg: cfg}
}
```

### After: local variable for normalization

```go
func NewPusher(cfg Config) *Pusher {
    registryURL := cfg.Registry
    if !strings.HasPrefix(registryURL, "http://") && !strings.HasPrefix(registryURL, "https://") {
        registryURL = "https://" + registryURL
    }
    return &Pusher{
        cfg: Config{
            Registry: registryURL,
            Username: cfg.Username,
            Password: cfg.Password,
            Token:    cfg.Token,
            Insecure: cfg.Insecure,
        },
    }
}
```

## Related

- [Cobra CLI scaffolding with test-first flag validation and credential fallback](cobra-cli-scaffolding.md) — companion doc covering the CLI credential fallback chain and docker-config loading entry point
- [Pure Go registry push from Docker distribution v2 on-disk layout](../design-patterns/pure-go-registry-push-docker-v2-on-disk-layout.md) — the registry push design pattern that consumes the auth config

## Commits

- Implementation: `c50aae3` — "fix(registry): wire bearer-token auth and surface docker-config errors"
- Review fixes: `09c2920` — "fix(auth): surface docker-config errors and improve auth robustness"

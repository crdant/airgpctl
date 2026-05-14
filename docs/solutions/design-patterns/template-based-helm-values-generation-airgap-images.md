---
title: "Template-based Helm values generation for air-gapped image registries"
date: 2026-05-14
category: design-patterns
module: pkg/values
problem_type: design_pattern
component: testing_framework
severity: medium
applies_when:
  - "Generating Helm values files that map container images to private registry paths"
  - "Needing configurable image path templates with variable substitution"
  - "Filtering bundle images against chart references for partial matching"
tags:
  - helm-values
  - template-substitution
  - image-mapping
  - chart-aware-filtering
  - test-first
  - red-green-refactor
---

# Template-based Helm values generation for air-gapped image registries

## Context

When loading Replicated `.airgap` bundles into a private registry, users need a Helm values file that maps each original image reference to its new destination path. The mapping must be:
- **Configurable**: registry host, namespace prefix, and tag strategy vary per environment
- **Chart-aware**: only images referenced by the target Helm chart should appear in the values file
- **Safe**: duplicate image names from different repositories must be detected rather than silently overwritten

## Guidance

### Test-first with RED-GREEN-REFACTOR

Implement values generation test-first, one test at a time:
1. Write a failing test that asserts the desired output
2. Implement the minimal code to make it pass
3. Refactor for clarity while keeping tests green
4. Repeat for each feature slice

This approach forces the API design to emerge from real usage scenarios rather than speculative abstraction.

### Template variable substitution

Use simple string-based template substitution for image path generation. Support these variables:

- `{registry}` — destination registry host
- `{namespace}` — optional namespace/prefix
- `{name}` — short image name (last path component)
- `{tag}` — image tag (original or overridden)
- `{repository}` — full original repository path

Clean up edge cases after substitution:
- Empty namespace produces double slashes (`//`) — collapse to single slashes
- Trailing slash before tag separator (`/:`) — remove the slash
- Unknown template variables (`{unknown}`) — return a clear error

Example template and result:
```
Template:  "{registry}/{namespace}/{name}:{tag}"
Registry:  "myregistry.example.com"
Namespace: "prod"
Repo:      "library/nginx"
Tag:       "latest"
Result:    "myregistry.example.com/prod/nginx:latest"
```

### Chart-aware filtering with partial matching

Implement filtering as a pluggable `func(string) bool` so callers can supply custom logic. For chart-aware matching, use bidirectional substring containment:

```go
func (c *ChartMatcher) Match(name string) bool {
    for _, ref := range c.refs {
        if strings.Contains(name, ref) || strings.Contains(ref, name) {
            return true
        }
    }
    return false
}
```

This catches:
- Exact matches (`"nginx"` matches `"nginx"`)
- Composite names (`"nginx"` matches `"nginx-plus"`)
- Namespaced references (`"my-nginx"` matches `"nginx"`)

**Caveat**: False positives can occur (e.g., `"redis"` matches `"predis"`). Document this behavior and accept it for an MVP; refine with exact or regex matching if needed later.

### Duplicate detection in flat YAML maps

When using short image names as YAML keys, two repositories can collide (e.g., `library/nginx` and `bitnami/nginx` both map to key `"nginx"`). Detect this explicitly and return an error rather than silently overwriting:

```go
if _, exists := entries[name]; exists {
    return fmt.Errorf("duplicate image name %q from repository %q", name, img.Repository)
}
```

This prevents subtle configuration bugs where the wrong image gets deployed.

## Why This Matters

- **Configurability without complexity**: String templates are simple, fast, and require no external dependency — critical for a single static binary in air-gapped environments
- **Testability**: Table-driven tests with temporary directories verify both the generated YAML structure and file I/O behavior
- **Safety**: Explicit errors for duplicates and unknown template variables prevent silent misconfiguration
- **Separation of concerns**: Filtering is a function, not baked into the generator, allowing chart-aware matching, allowlists, or other strategies without changing core generation logic

## When to Apply

- Any Go CLI that needs to generate YAML configuration with variable substitution
- Image mapping where source and destination registries differ
- Filtering a large set of artifacts against a smaller reference set (partial matching)

## Examples

### Basic generation for all images

```go
g := &values.Generator{
    Registry:  "myregistry.example.com",
    Namespace: "prod",
    Template:  "{registry}/{namespace}/{name}:{tag}",
}

err := g.Generate(images, "images.yaml")
```

### Chart-aware filtering (only nginx and redis)

```go
matcher := values.NewChartMatcher([]string{"nginx", "redis"})
g := &values.Generator{
    Registry:  "myregistry.example.com",
    Namespace: "prod",
    Template:  "{registry}/{namespace}/{name}:{tag}",
    Filter:    matcher.Match,
}

err := g.Generate(images, "images.yaml")
```

### Tag override (all images get "stable")

```go
g := &values.Generator{
    Registry:    "myregistry.example.com",
    TagOverride: "stable",
}
```

## Related

- `docs/solutions/design-patterns/safe-tar-extraction-yaml-parsing-airgap-bundles.md` — prior session covering bundle extraction and YAML parsing patterns
- `pkg/values/values.go` — implementation
- `pkg/values/values_test.go` — 13 table-driven tests covering happy path, edge cases, error paths, and helper functions

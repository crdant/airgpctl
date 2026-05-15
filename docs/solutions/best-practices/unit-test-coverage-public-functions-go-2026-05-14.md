---
title: "Unit-test coverage patterns for public functions in Go"
date: 2026-05-14
category: best-practices
module: unit-testing
problem_type: best_practice
component: testing_framework
severity: medium
applies_when:
  - "Adding or reviewing unit tests for public Go functions"
  - "Writing test helpers that perform setup or I/O"
  - "Asserting error behavior in Go tests"
tags:
  - go
  - testing
  - unit-tests
  - coverage
  - public-functions
  - test-helpers
  - yaml
  - negative-tests
  - error-assertions
---

# Unit-test coverage patterns for public functions in Go

## Context

During U7 ("Add unit-test coverage for new public functions"), the initial implementation commit (`210755f`) added tests for several newly exported functions. Code review surfaced five recurring quality gaps that make failures harder to diagnose and leave production code under-protected. A follow-up commit (`b56192c`) fixed all five patterns.

## Guidance

### 1. Fail fast in test helpers with `t.Fatalf`

When a test helper performs setup that subsequent steps depend on (creating temp files, encoding data, starting listeners), any error must stop the test immediately. Returning or ignoring the error produces secondary failures that appear unrelated to the root cause, wasting debugging time.

```go
// Bad — helper silently drops errors; test panics later with a confusing stack trace
func writeValuesFile(t *testing.T, dir string, vals map[string]interface{}) string {
    p := filepath.Join(dir, "values.yaml")
    b, _ := yaml.Marshal(vals)
    os.WriteFile(p, b, 0644)
    return p
}

// Good — helper fails fast with a clear message
func writeValuesFile(t *testing.T, dir string, vals map[string]interface{}) string {
    t.Helper()
    p := filepath.Join(dir, "values.yaml")
    b, err := safeYAMLMarshal(vals)
    if err != nil {
        t.Fatalf("marshaling values: %v", err)
    }
    if err := os.WriteFile(p, b, 0644); err != nil {
        t.Fatalf("writing values file: %v", err)
    }
    return p
}
```

### 2. Guard `yaml.Marshal` with `recover()` for unmarshalable types

`gopkg.in/yaml.v3` (and `yaml.v2`) can panic when asked to marshal values it cannot serialize, such as channels, functions, or certain cyclic structures. Production code that accepts arbitrary Go values and serializes them must convert panics into clean errors rather than crashing the process.

```go
func safeYAMLMarshal(v interface{}) (out []byte, err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("yaml marshal panic: %v", r)
        }
    }()
    out, err = yaml.Marshal(v)
    return
}
```

### 3. Always add negative tests for error paths and invalid inputs

Coverage that only exercises happy paths does not prove the code handles corruption, missing files, or bad data. For every function that returns `(T, error)`, add at least one test case that:

- Supplies malformed, missing, or out-of-range input.
- Verifies the function returns a non-nil error.
- Where possible, asserts that the returned error is the specific one expected.

### 4. Assert error message content, not just `err != nil`

A non-nil error is necessary but not sufficient. Two different error paths may both return `errors.New("something went wrong")`, hiding a regression. Use `strings.Contains`, `errors.Is`, or dedicated error types to assert the specific failure mode.

```go
// Bad — any non-nil error satisfies this, so a wrong error path looks correct
if err == nil {
    t.Fatal("expected error")
}

// Good — asserts the specific failure
if !strings.Contains(err.Error(), "no such file") {
    t.Fatalf("expected 'no such file' error, got: %v", err)
}
```

### 5. Target coverage gaps on public functions first

Public APIs are the highest-signal targets for test-first work because they are the contracts other packages depend on. Before adding exhaustive tests for private helpers, ensure every exported function has at least a happy-path and a negative-path test. This produces the greatest defect-prevention value per line of test code.

## Why This Matters

- **Confusing failures cost more than missing tests**: A helper that silently drops an error turns a 30-second fix into a 10-minute debugging session.
- **Panic safety in production**: Unmarshaling user-provided or generated data into YAML without recovery can crash the process.
- **False confidence from coverage metrics**: 90% line coverage that skips all error branches is less valuable than 60% coverage that exercises both success and failure paths.
- **Public-function tests document intent**: They serve as executable specifications for consumers of the package.

## When to Apply

- Adding unit tests for newly exported functions.
- Reviewing test PRs that only cover happy paths.
- Writing shared test helpers in `*_test.go` files.
- Serializing user-provided or generated data to YAML or JSON in production code.

## Examples

### Before/after: test helper error handling

```go
// Before (initial commit 210755f)
func writeValuesFile(t *testing.T, dir string, vals map[string]interface{}) string {
    p := filepath.Join(dir, "values.yaml")
    b, _ := yaml.Marshal(vals) // silent error, panics on channels
    os.WriteFile(p, b, 0644)
    return p
}

// After (review fix commit b56192c)
func writeValuesFile(t *testing.T, dir string, vals map[string]interface{}) string {
    t.Helper()
    p := filepath.Join(dir, "values.yaml")
    b, err := safeYAMLMarshal(vals)
    if err != nil {
        t.Fatalf("marshaling values: %v", err)
    }
    if err := os.WriteFile(p, b, 0644); err != nil {
        t.Fatalf("writing values file: %v", err)
    }
    return p
}
```

### Before/after: negative test coverage

```go
// Before — only happy path
func TestParseValues(t *testing.T) {
    got, err := ParseValues("valid.yaml")
    if err != nil {
        t.Fatal(err)
    }
    // ... assertions on got
}

// After — happy + negative paths
func TestParseValues(t *testing.T) {
    t.Run("valid file", func(t *testing.T) {
        got, err := ParseValues("valid.yaml")
        if err != nil {
            t.Fatal(err)
        }
        // ... assertions on got
    })
    t.Run("missing file", func(t *testing.T) {
        _, err := ParseValues("/nonexistent/values.yaml")
        if err == nil {
            t.Fatal("expected error for missing file")
        }
        if !strings.Contains(err.Error(), "no such file") {
            t.Fatalf("unexpected error message: %v", err)
        }
    })
    t.Run("corrupt yaml", func(t *testing.T) {
        // ... assert specific parse error
    })
}
```

## Related

- `docs/solutions/best-practices/dry-gzip-tar-detection-test-helpers-edge-cases-2026-05-14.md` — DRY principle and test-helper deduplication (related but distinct focus on helper structure rather than coverage strategy)
- `docs/solutions/best-practices/image-reference-parsing-testing-conventions-2026-05-14.md` — table-driven test naming conventions

---
title: Assert exact error message text and keep test names in sync
date: 2026-05-14
category: conventions
module: bundle
problem_type: convention
component: testing_framework
severity: low
applies_when:
  - "Table-driven tests validate error paths"
  - "Error messages are part of the public API or user-facing output"
  - "Error message text is changed or corrected"
tags:
  - testing
  - table-driven-tests
  - error-messages
  - wantErr
---

# Assert exact error message text and keep test names in sync

## Context

When changing error message text in `pkg/bundle/bundle.go` (even a trivial one-line capitalization fix changing `"SavedImages"` to `"savedImages"`), the existing table-driven test only checked `wantErr: true`. This allowed the error text to drift silently without failing tests. The code review also caught that the test case name was not updated to match the new error message text, creating a mismatch between the test description and the actual behavior.

## Guidance

When testing error paths in table-driven tests:

1. **Assert on the exact error message text**, not just `wantErr: true`. Add a `wantErrMsg` field to the test struct and verify the returned error contains the expected substring.
2. **Keep test case names in sync** with error message text changes. If an error message says `"missing savedImages field"`, the test case name should say `"missing savedImages field"`, not `"missing SavedImages field"`.

## Why This Matters

Error messages are part of the user-facing API. Silent drift between test names and actual error text makes debugging harder and indicates the tests are not actually validating the behavior they claim to test. Without exact error message assertions, regressions in error text go unnoticed.

## When to Apply

- Any table-driven test with `wantErr: true`
- When modifying error message strings
- When reviewing PRs that change user-facing error text

## Examples

**Before:**

```go
{
    name: "missing SavedImages field",
    content: `spec:
  otherField: "value"
`,
    wantImages: nil,
    wantErr:    true,
},
```

**After:**

```go
{
    name:       "missing savedImages field",
    content:    `spec:
  otherField: "value"
`,
    wantImages: nil,
    wantErr:    true,
    wantErrMsg: "savedImages",
},
```

And in the test execution loop:

```go
if tt.wantErr {
    if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
        t.Errorf("OpenBundle() error = %v, want error containing %q", err, tt.wantErrMsg)
    }
    return
}
```

## Related

- [Unit test coverage for public functions in Go](../best-practices/unit-test-coverage-public-functions-go-2026-05-14.md)

---
title: Centralize registry HTTP error classification and response body handling
date: 2026-05-14
category: conventions
module: pkg/registry
problem_type: convention
component: service_object
severity: medium
applies_when:
  - Writing or modifying registry push client code
  - Adding new registry interaction phases
  - Reviewing HTTP response handling for connection reuse
tags:
  - registry
  - http
  - error-handling
  - connection-reuse
  - go
---

# Centralize registry HTTP error classification and response body handling

## Context

During U6 ("Improve registry HTTP error classification"), the initial implementation added explicit HTTP status classification only to HEAD checks (`manifestExists` and `pushBlob` existence checks). Code review found that POST (blob upload start), PUT (blob upload complete), and PUT (manifest push) still fell back to generic "unexpected status" messages. This meant an operator hitting a 503 during manifest push would see `unexpected status 503 pushing manifest` instead of a clear retriable error, and a 401 during blob upload start would not be identified as an auth failure.

In addition, response body handling was inconsistent: some paths closed the body inline (`resp.Body.Close()`) while others deferred it. For Go's HTTP connection reuse, the body must be fully drained before closing.

## Guidance

### 1. Centralize status-code mapping in a single helper

Extract a `registryError(statusCode int, context string) error` helper that maps codes to actionable errors:

- `401`/`403` → `registry authentication failed: <context>`
- `500`/`502`/`503`/`504` → `registry temporarily unavailable (status <code>): <context>`
- Everything else → `unexpected status <code> <context>`

Call this helper from every registry interaction (HEAD, POST, PUT) so classification is uniform across all push phases.

### 2. Use `defer resp.Body.Close()` consistently

Always defer the response body close immediately after the `client.Do` error check. This prevents leaks on early returns.

### 3. Drain the body before the deferred close when connection reuse matters

If the body is not otherwise read (e.g. after checking only `StatusCode`), drain it with `io.Copy(io.Discard, resp.Body)` before the function returns and the deferred close runs. The drain can be done inline right after the defer:

```go
defer resp.Body.Close()
io.Copy(io.Discard, resp.Body)
```

If you consume the body fully (e.g. `json.NewDecoder(resp.Body).Decode(&v)`), no extra drain is needed.

## Why This Matters

In air-gapped deployments, registry errors are often the first sign of network or auth misconfiguration. Generic "unexpected status 502" messages force operators to read source code to decide whether to retry or fix credentials. Consistent classification gives immediate actionable guidance. Connection reuse matters because pushing large bundles opens many HTTP connections; failing to drain and close bodies leaks connections and slows subsequent pushes.

## When to Apply

- Writing or modifying code that makes HTTP requests to a Docker registry
- Adding new push phases (manifest lists, chunked upload, cross-repo mount)
- Reviewing registry client code for error message clarity or connection pool health

## Examples

### Before: duplicated switch logic in every function

```go
switch resp.StatusCode {
case http.StatusOK:
    return true, nil
case http.StatusNotFound:
    return false, nil
case http.StatusUnauthorized, http.StatusForbidden:
    return false, fmt.Errorf("registry authentication failed: %s", url)
case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
    return false, fmt.Errorf("registry temporarily unavailable (status %d): %s", resp.StatusCode, url)
default:
    return false, fmt.Errorf("unexpected status %d from HEAD %s", resp.StatusCode, url)
}
```

### After: centralized helper used everywhere

```go
func registryError(statusCode int, context string) error {
    switch statusCode {
    case http.StatusUnauthorized, http.StatusForbidden:
        return fmt.Errorf("registry authentication failed: %s", context)
    case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
        return fmt.Errorf("registry temporarily unavailable (status %d): %s", statusCode, context)
    default:
        return fmt.Errorf("unexpected status %d %s", statusCode, context)
    }
}

// usage in manifestExists
return false, registryError(resp.StatusCode, "from HEAD "+url)

// usage in pushBlob upload start
return registryError(resp.StatusCode, "starting blob upload")
```

### Before: inline body close, no drain

```go
resp, err := p.client.Do(req)
if err != nil {
    return err
}
resp.Body.Close()
```

### After: defer + drain for connection reuse

```go
resp, err := p.client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()
io.Copy(io.Discard, resp.Body)
```

## Related

- [Registry auth and docker-config error handling: surfacing failures and parsing edge cases](../best-practices/registry-auth-docker-config-error-handling-2026-05-14.md) — companion doc covering credential loading and image reference parsing
- [Pure Go registry push from Docker distribution v2 on-disk layout](../design-patterns/pure-go-registry-push-docker-v2-on-disk-layout.md) — the registry push design pattern that uses these HTTP conventions

## Commits

- Implementation: `34c1290` — "fix(registry): classify HTTP errors during push (U6)"
- Review fixes: `e3b370c` — "fix(registry): complete HTTP error classification across all push phases"

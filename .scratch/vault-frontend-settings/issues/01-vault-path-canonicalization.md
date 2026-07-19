---
title: "Canonicalize vault paths and derive a reproducible cache key"
created: 2026-07-19
tags: [issue]
---

Status: resolved
Type: task

## Context

Read `docs/adr/0011-frontend-driven-vault-selection.md` before starting. The per-vault Engines cache is moving to `~/.cache/ks/<hash>/`, where `<hash>` must be reproducible for the *same physical vault* regardless of how its path is written (`~/knowledge`, `/home/user/knowledge`, a symlink to either). No canonicalization or hashing helper exists anywhere in this repo today (confirmed: no `EvalSymlinks`, no `filepath.Abs`, no hashing in `internal/vault`) — this is new code, not a refactor.

Tilde expansion currently lives inline (not exported) in `internal/config/config.go:41-47` — that whole file is being deleted in Ticket 04/09, so do not depend on it; write fresh, exported logic in `internal/vault`.

## What to implement

In `internal/vault` (new file, e.g. `internal/vault/canonical.go`):

```go
// CanonicalPath expands a leading "~", makes the result absolute, and
// resolves symlinks, so the same physical vault always canonicalizes
// identically regardless of how its path was written.
func CanonicalPath(path string) (string, error)

// CacheKey returns a reproducible identifier for a canonicalized vault
// path, suitable for use as a directory name under ~/.cache/ks/.
func CacheKey(canonicalPath string) string
```

- `CanonicalPath`: expand `~` the same way `internal/config/config.go:41-47` did (read that code now, before it's deleted, to match the existing tilde-handling behavior exactly — e.g. does it only handle a bare `~` prefix, or `~user`? Match existing scope, don't expand it), then `filepath.Abs`, then `filepath.EvalSymlinks`. Return a wrapped error if any step fails (e.g. path doesn't exist yet for `EvalSymlinks` — decide whether that's an error here or deferred to `ValidateRoot`, which already runs separately at switch time per Ticket 05).
- `CacheKey`: SHA-256 of the canonical path string, hex-encoded, truncated to a reasonable length (16-32 hex chars is plenty for a directory name — full 64 is unnecessary noise). Pure function, no I/O.

## Verification checklist

- New tests in `internal/vault/canonical_test.go`, following the existing test file's conventions (`internal/vault/vault_test.go` uses a `writeFile` helper and `t.TempDir()` — reuse that style).
- Cover: `~` expansion produces an absolute path; two different string forms of the same real directory (e.g. via a symlink created with `os.Symlink` in the test) produce the *same* `CacheKey`; two genuinely different directories produce different keys; a nonexistent path either errors clearly or is documented as deferring validation elsewhere (pick one, be explicit in a doc comment).
- `go test ./internal/vault/...` passes.
- `go vet ./...` clean.

## Anti-pattern guards

- Don't use `filepath.Clean` as a substitute for `EvalSymlinks` — it doesn't resolve symlinks.
- Don't silently swallow `EvalSymlinks` errors — a cache-key function that sometimes returns different keys for the same vault (because resolution failed and it fell back to the raw path) defeats the entire point.

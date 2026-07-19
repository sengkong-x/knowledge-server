---
title: "Active Vault manager: switch, save-on-switch, trust-then-reconcile"
created: 2026-07-19
tags: [issue]
---

Status: resolved
Type: task

## Context

This is the core new subsystem. Depends on Tickets 01 (canonicalization/cache key), 02 (cache versioning), 03 (settings.json) existing with the signatures described in those tickets — read them, don't guess.

Read first: `docs/adr/0011-frontend-driven-vault-selection.md`, the 2026-07-19 update block in `docs/adr/0010-cache-persistence-lifecycle.md`, and the **Active Vault** entry in `CONTEXT.md`.

Key existing facts (from Phase 0 discovery, re-confirm by reading if anything seems off):
- `state.State` (`internal/state/state.go:18-24`) wraps exactly **one** `*engines.Engines` and has **no existing method to swap it** — `New(e *engines.Engines) *State` only constructs.
- `watcher.Watcher` (`internal/watcher/watcher.go`) is constructed fresh per root via `New(root string, targets ...Target) (*Watcher, error)`; `Close()` (`:98`) fully drains before returning (documented ordering at `:91-97`) — confirmed mechanically safe to `Close()` then `New()` again for a different root, but no existing code does this sequence.
- `internal/server/server.go`'s `New(vaultPath, provider, store, s *state.State, theme string) http.Handler` captures all five params as **immutable closure values** in ~14 route handlers (`server.go:154-400`) — there is no existing way to re-point a live handler at a new vault. This ticket must produce something Ticket 06 can plug in *without* reconstructing the whole handler on every switch (reconstructing per switch is possible but means re-registering all routes and swapping `httpServer.Handler`, which races with in-flight requests unless guarded — prefer the indirection approach below).

## What to implement

New package `internal/activevault` (name deliberately matches the `CONTEXT.md` glossary term — don't call it `vaultmanager` or `session`):

```go
type ActiveVault struct {
    mu       sync.RWMutex
    path     string             // canonical path, "" if none selected
    provider vault.VaultProvider
    store    notes.NoteStore
    state    *state.State
    theme    string
    watcher  *watcher.Watcher
}

func New(theme string) *ActiveVault // starts with no vault selected

// Snapshot is what request handlers read per-request — copy the fields
// they need under RLock so a concurrent Switch can't hand back a half-updated
// view. Return ok=false if no vault is currently selected.
func (av *ActiveVault) Snapshot() (path string, provider vault.VaultProvider, store notes.NoteStore, s *state.State, ok bool)

func (av *ActiveVault) Theme() string
func (av *ActiveVault) SetTheme(theme string) // no vault dependency, just swaps the value under lock

// Switch validates newPath, saves+discards the outgoing vault (if any),
// loads-or-builds the incoming vault's Engines from its ~/.cache/ks/<hash>/
// cache dir (instant if present — "trust cache immediately" per the grill),
// swaps in new provider/store/state/watcher under the write lock, and
// returns a channel/callback the caller can use to know when a background
// staleness reconciliation finishes (see below). Returns an error the HTTP
// layer (Ticket 06) surfaces to the picker UI on validation failure —
// the outgoing vault must NOT be torn down if the new path fails ValidateRoot.
func (av *ActiveVault) Switch(newPath string) error

// Reconcile is called internally by Switch (in a goroutine) after the
// switch completes: rescans the vault for staleness (see below) and, if
// anything changed since the cache was written, drives Upsert/Remove calls
// through the new State — which already pings existing subscribers via
// State's own Upsert/Remove (state.go), so this reuses the existing SSE
// plumbing (server.go:216-244) with zero changes needed there.
```

### Switch sequence (this is the load-bearing logic)

1. `canonical, err := vault.CanonicalPath(newPath)` (Ticket 01) — error surfaces immediately, no state touched.
2. `vault.ValidateRoot(canonical)` — same, error surfaces immediately.
3. Take write lock. If a vault is currently active: `av.state.Save(currentCachePaths)` (extends ADR-0010's trigger, per the 2026-07-19 update — this is a **second** call site for the exact same `Save` semantics, not new save logic), then `av.watcher.Close()`.
4. Compute `cacheDir := filepath.Join(userCacheDir(), "ks", vault.CacheKey(canonical))` (Ticket 01's `CacheKey`), `os.MkdirAll`.
5. `cachePaths := engines.Paths{Index: filepath.Join(cacheDir, "index.gob"), Search: ..., Graph: ...}` — same three filenames as today (`cmd/main.go:52-56`), just a different parent dir.
6. `e, report, err := engines.LoadOrBuild(cachePaths, provider, store)` — per Ticket 02, a version-mismatched or missing cache transparently falls back to `Build()`, so this single call already implements "trust cache if present, else build."
7. Construct new `provider`, `store`, `state.New(e)`, `watcher.New(canonical, newState)`, `.Start()`. Swap all of `av.path/provider/store/state/watcher` under the still-held write lock. Release lock.
8. Log any `report.*.Failed` warnings (mirror `cmd/main.go:63-71`'s existing pattern with the same `log.Warn` convention).
9. Kick off `go av.reconcile(canonical, cachePaths)` — this is the "background rescan" from the grill's Q10/A decision: even when the cache was loaded (not built), re-derive a cheap signal of whether the vault changed while it wasn't being watched (e.g. compare `provider.ListNotes()` count/mtimes against what's in the freshly loaded `Index` — reuse `notes.NoteStore`/`vault.VaultProvider` methods already in use elsewhere, don't invent new filesystem-walking code). If stale, call `Upsert`/`Remove` through the *new* `State` for whatever changed — the existing `/events` SSE endpoint (`server.go:216-244`) already notifies subscribers on any `State.Upsert`/`Remove`, so no server.go change is needed for the live-update part of this.

### Settings integration

`Switch` itself does **not** call `settings.Save` — keep this package settings-agnostic (it doesn't know about `~/.config`). Ticket 06's HTTP handler calls `activevault.Switch(path)`, and on success calls `settings.Save(settings.Load().WithVault(path))` (Ticket 03) — persistence is the HTTP layer's job, switching is this package's job. Same split for `SetTheme`.

## Verification checklist

- New tests in `internal/activevault/activevault_test.go`. Cover: `Switch` to a valid new vault succeeds and `Snapshot()` reflects it; `Switch` to an invalid path returns an error and leaves the previously-active vault untouched (`Snapshot()` still returns the old one); switching A→B→A reuses A's cache (assert via a fast path — e.g. instrument or time-bound, or simply assert the reloaded `Index` matches what was there before without re-parsing from scratch, whatever's practically testable given `LoadOrBuild`'s existing test patterns in `internal/engines/engines_test.go`); concurrent `Snapshot()` calls during a `Switch` don't panic or return a half-swapped state (run with `-race`).
- `go test -race ./internal/activevault/...` passes.
- No vault selected at construction: `Snapshot()` returns `ok=false`, doesn't panic.

## Anti-pattern guards

- Don't let `Switch` mutate `av`'s fields before validation succeeds — an invalid new path must never tear down a perfectly good outgoing vault.
- Don't keep every visited vault's `Engines`/`Watcher` resident — Q11/A of the grill explicitly rejected that (unbounded memory growth); exactly one vault's subsystem is live at a time.
- Don't call `settings.Save` from inside this package — keep the layering the ticket describes.
- Don't reinvent staleness detection as a full re-`Build()` on every switch — that defeats the entire point of caching; reconciliation should be cheap-check-first, matching the grill's Q10/A decision (trust cache, reconcile in background), not Q10/B's "validate before serving."

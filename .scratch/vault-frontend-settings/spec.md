---
title: "Spec: config.yaml removal, frontend-driven vault selection, frontend modernization"
created: 2026-07-19
tags: [spec]
---

Status: ready

## Origin

Grilled and confirmed with the user on 2026-07-19; decisions recorded in `docs/adr/0011-frontend-driven-vault-selection.md` and the 2026-07-19 update to `docs/adr/0010-cache-persistence-lifecycle.md`. Glossary terms **Active Vault** and **Settings** added to `CONTEXT.md`. This spec does not re-litigate those decisions — implementers should read the ADRs for rationale, not question the "why."

## Scope (from the grill)

1. Delete `internal/config`, `config.yaml`, `docs/config.yaml`, and the `--config` flag entirely.
2. `server.port` → `--port` CLI flag (default `8080`, stdlib `flag`).
3. No `vault.path`/`theme` startup argument. Server boots with **no Active Vault**.
4. Settings subsystem: `~/.config/ks/settings.json` — `{vault_path, theme, vault_history}` — loaded at boot (auto-reopens last vault if present), written on every vault switch and theme change.
5. Per-vault Engines cache relocated to `~/.cache/ks/<hash>/`, `<hash>` = SHA-256 of the canonicalized (tilde-expanded, absolute, symlink-resolved) vault path. Keep gob (ADR-0004) with a version marker so a format mismatch is treated as a cache miss, not a decode panic/silent garbage.
6. Vault switch: validate new path (`ValidateRoot`), save + discard outgoing vault's `Engines` (extends ADR-0010's save trigger), instantly serve cached `Engines` for the incoming vault if present, else build from scratch — either way kick off a background staleness rescan that live-updates over the existing SSE (`/events`) plumbing.
7. New HTTP endpoints: get current settings/vault history, switch active vault (surfacing validation errors), switch theme (global, immediate).
8. Frontend vault picker: dropdown of `vault_history` + "Add new vault..." revealing a free-text path input.
9. Frontend visual modernization: stays zero-build-step (hand-written CSS/JS + existing vendored htmx/alpine/cytoscape, Go `embed`). No npm/bundler/Tailwind.
10. Update `README.md` for the new CLI flag and first-run vault-picker flow.

## Phase 0: Documentation Discovery — Allowed APIs

Confirmed by direct source reads (`internal/engines/engines.go`, `internal/index/index.go`, `internal/search/search.go`, `internal/graph/graph.go`, `internal/vault/vault.go`, `internal/state/state.go`, `internal/watcher/watcher.go`, `internal/server/server.go`, `internal/logger/logger.go`, `cmd/main.go`, `web/`).

```go
// internal/engines
type Paths struct { Index, Search, Graph string }
func LoadOrBuild(paths Paths, provider vault.VaultProvider, store notes.NoteStore) (*Engines, BuildReport, error)
func Build(provider vault.VaultProvider, store notes.NoteStore) (*Engines, BuildReport, error)
func (e *Engines) SaveAll(paths Paths) error

// internal/vault
func ValidateRoot(root string) error
func NewLocalVaultProvider(root string) VaultProvider
// NOTE: no canonicalization/hashing helper exists yet — this spec requires adding one.

// internal/state
func New(e *engines.Engines) *State
func (s *State) Save(paths engines.Paths) error
func (s *State) Subscribe() (ch <-chan struct{}, unsubscribe func())
// State wraps exactly one Engines value (state.go:18-24) — no existing API to swap it.

// internal/watcher
func New(root string, targets ...Target) (*Watcher, error)
func (w *Watcher) Start() error
func (w *Watcher) Close() error
// Close() fully drains (watcher.go:91-97) — mechanically safe to Close() then New() again,
// but no existing code does this; it's new call-sequence territory.

// internal/server
func New(vaultPath string, provider vault.VaultProvider, store notes.NoteStore, s *state.State, theme string) http.Handler
// Called exactly once today (cmd/main.go:85). All params captured as immutable closure
// values in every route handler — there is no existing mechanism to re-point a live
// server.Handler at a new vault/state/theme. A vault/theme switch needs either (a) new
// mutable state server.New closes over by reference, or (b) reconstructing the handler.

// internal/logger
func New() *slog.Logger
// convention: log.<Level>("lowercase message", "key", val, ...) — Info/Warn/Error only.
```

Cache file names in use today (`cmd/main.go:52-56`): `index.gob`, `search.gob`, `graph.gob` inside a cache dir — these names carry forward unchanged, only the containing directory moves.

`gob.Encode`/`gob.Decode` today operate directly on the raw map for index/search, and on a `graphData{Entries: map}` wrapper for graph (`internal/index/index.go:68`, `internal/search/search.go:140`, `internal/graph/graph.go:177`) — **no version header exists**. `LoadOrBuild` in all three packages treats any `Load` error (missing file *or* corrupt/incompatible decode) identically: fall back to `Build()` (index.go:122-127, search.go:83-88, graph.go:41-46). This spec's cache-versioning work must preserve that "any problem → rebuild, never panic/garbage" behavior — do not weaken it.

### Anti-patterns to avoid

- Do **not** invent a `server.SetVault(...)`/`server.Rebind(...)` method — it doesn't exist. Either wrap the swappable state behind a small indirection struct closed over by `server.New`, or reconstruct the `http.Handler` and swap it on the running `*http.Server` (`httpServer.Handler = newHandler` — safe between requests only if guarded; confirm locking approach in Ticket 05).
- Do **not** add a version *field* to the existing `IndexEntry`/`searchEntry`/`graphData` structs and call that "versioning" — gob is largely tolerant of added struct fields, so this would NOT reliably reject old-format files. Use an explicit magic/version header written and checked *before* the gob stream (see Ticket 02 for the exact approach).
- Do **not** reach for `filepath.Clean` alone as "canonicalization" — it doesn't expand `~` or resolve symlinks, so two different physical vault directories (or the same one reached two ways) could hash differently or identically by accident.
- Do **not** add npm/Tailwind/a bundler for the frontend phase (Ticket 08) — explicitly out of scope per the grill.

## Phase ordering / dependencies

```
01 (canonicalization + cache key)  →  02 (cache versioning)  →  03 (settings.json)
                                                                        │
04 (CLI flags, delete internal/config, main.go reboot) ────────────────┤
                                                                        ▼
                                              05 (Active Vault switch orchestration)
                                                                        │
                                                                        ▼
                                              06 (HTTP endpoints for settings/switch/theme)
                                                                        │
                                                                        ▼
                                              07 (frontend vault picker UI)
                                                                        │
                                                                        ▼
                                              08 (frontend visual modernization)

09 (README + delete config.yaml/docs/config.yaml) — can run anytime after 04
10 (final verification) — last
```

Tickets 01-03 can be done in parallel by different sessions since they're independent packages; 04 depends on the *signatures* of 01-03 existing (doesn't need their full implementation reviewed, just the function signatures agreed). Each ticket below is self-contained — read it fresh, don't assume the implementer has this spec's full context loaded.

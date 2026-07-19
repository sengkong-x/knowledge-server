---
title: Load cache at startup, save only on graceful shutdown
created: 2026-07-18
updated: 2026-07-19
tags: [adr]
---

`Index`, `SearchStore`, and `Graph` have had `Save`/`Load` persistence (ADR-0004's gob encoding) since Ticket 03/04/05, but `cmd/main.go` has never called them — every startup does a full `Build()` from the Vault. Ticket 06 makes the server long-running with continuous Watcher-driven mutation, which makes startup cost worth addressing. We considered leaving persistence unwired (startup cost is orthogonal to live-editing UX) against wiring it up now, and chose to wire it up: the APIs already exist and sit unused, and a long-running server restarting after a deploy or crash is exactly the scenario they were built for.

For when `Save()` fires, we considered saving on every `Upsert`/`Remove` (fully durable, but each `Save()` is a whole-structure gob dump per ADR-0004, so this would turn every single note edit into a full-structure disk write), a periodic snapshot timer (bounds worst-case loss but adds a background ticker), and saving only on graceful shutdown (`SIGINT`/`SIGTERM`). We chose graceful-shutdown-only: it matches the project's "disposable, rebuildable" philosophy — losing the cache on a hard crash isn't data loss, since the Vault is always the source of truth and `Build()` fully reconstructs it, just a slower next startup. This requires adding signal handling to `main.go`, which doesn't exist today.

## Consequences

A hard kill (`kill -9`, power loss, OOM kill) loses the cache and the next startup pays a full `Build()`. This is an accepted, recoverable cost, not a bug — if crash frequency ever makes full rebuilds a real pain point, add a periodic snapshot on top of graceful-shutdown save rather than replacing it.

**Update (2026-07-19, see ADR-0011):** now that a running instance can switch between vaults at runtime, "shutdown" is no longer the only point at which a vault's engines stop being current. Switching away from a vault also triggers `Save()` for the outgoing vault. This is the same `Save()` call, just with a second trigger point — not a new mechanism. The alternative, keeping every visited vault's `Engines` resident in memory for the life of the process, was rejected: memory would grow unbounded with the number of distinct vaults opened in a session.

The incoming vault is loaded/built *before* the outgoing vault is saved and discarded, not after — the reverse of the naive "tear down old, then bring up new" order. `internal/activevault.ActiveVault.Switch` validates the new path, then loads-or-builds the incoming vault's engines and starts its watcher entirely outside the write lock; only once that has succeeded does it take the lock, save+discard the outgoing vault, and swap the incoming one in. Two considerations drove this: it keeps concurrent `Snapshot()` reads unblocked for the (potentially slow) duration of the incoming build rather than blocking them behind a held write lock, and it means a `LoadOrBuild`/watcher-start failure on the incoming vault leaves the outgoing vault completely untouched — not just a failed `ValidateRoot`, but any failure up through "the new vault is actually usable." The two watchers (outgoing, not yet closed; incoming, just started) briefly coexist for the few instructions between building the incoming one and taking the lock, which is harmless since each watches a disjoint filesystem root.

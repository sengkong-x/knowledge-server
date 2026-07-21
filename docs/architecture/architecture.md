---
title: Knowledge Server
created: 2026-07-18
tags: [architecture]
---

Knowledge Server is a self-hosted personal knowledge system: a single Go binary that reads, indexes, searches, and links notes stored as plain Markdown in a Vault. It is a personal "second brain" — continuous capture and reuse of knowledge from research, engineering work, architecture decisions, and learning notes — meant to stay human-readable and simple enough to maintain for decades.

The design rests on three principles. The **Vault is the source of truth**: everything the server derives from it — the Index, the SearchStore, the Graph, their on-disk cache — is disposable and must be fully rebuildable from the Vault at zero knowledge loss; deleting derived state is never a data-loss event, only a slower next startup. The system is organized around **engines, not features**: independent, single-responsibility components (Vault access, parsing, indexing, search, graph) each with a clean API, composed by a thin orchestration layer that holds no business logic of its own. And deployment is **minimal**: production is a binary plus a Vault directory — no database, no message queue, no container runtime required.

## C4 model

<!--
The image paths below (../assets/diagrams/...) are relative to this note's
own URL when served through the app (see "Viewing the documentation through
./ks" in the repo README): /notes/architecture resolves "../assets/..." to
/assets/..., which only works because this note's ID ("architecture") is a
single path segment. If this file is ever moved under a subdirectory of
docs/ (giving it a multi-segment ID, e.g. "guides/architecture"), these
paths need an extra "../" per added segment, or they'll 404 in that view.
-->

### Level 1 — System context

![System context diagram](../assets/diagrams/c4-context.svg)

A person manages their own knowledge by editing Markdown files directly (in their own editor) or through Knowledge Server's browser UI. Knowledge Server reads from and writes to the Vault — a directory of Markdown notes on disk that it does not own exclusively; it is designed to tolerate the Vault being edited out from under it at any time.

### Level 2 — Containers

![Container diagram](../assets/diagrams/c4-container.svg)

Inside the single Knowledge Server process:

- **HTTP / Web Server** — Go `net/http.ServeMux` (see ADR-0002), serving server-rendered HTML fragments (HTMX-swapped), a small JSON API, and the `/events` SSE stream. Holds no business logic; it reads from `State` and, for note content and Vault-wide listing, from `NoteStore`/`VaultProvider` directly.
- **State** — the process's in-memory data: the `Engines` (Index, SearchStore, Graph) behind a single mutex, plus the SSE subscriber registry. The one place concurrency is actually introduced (see ADR-0008).
- **Watcher** — an fsnotify-driven goroutine that watches the Vault directory recursively and drives debounced `Upsert`/`Remove` calls into `State` as files change.
- **Vault (disk)** — the Markdown notes and assets; authoritative, external to the process.
- **Cache (disk)** — `.cache/*.gob`, a gob-encoded snapshot of the Index, SearchStore, and Graph, deliberately kept outside the Vault directory so the Watcher doesn't treat its own writes as Vault edits.

### Level 3 — Components

![Component diagram](../assets/diagrams/c4-component.svg)

The dependency direction among `internal/` packages: `vault` → `notes` → `parser` for reading and parsing a Note; `notes` feeds `index`, `search`, and `graph` independently, which are composed by `engines` and guarded by `state`; `watcher` drives `state` (wired in `cmd/main.go` — `Watcher` depends only on its `Target` interface, satisfied by `*state.State`, not on the `state` package itself); `server` depends on `state`, `notes`, and `vault`. `index`, `search`, and `graph` also call back into `vault` directly for path resolution (`vault.ResolvePath`) and Vault-wide listing during a full `Build` — the dashed edges in the diagram.

## Request lifecycle: how a note edit reaches the browser

1. A note file is saved to the Vault (by the person's editor, or any other process writing into the Vault directory).
2. The **Watcher**'s fsnotify subscription fires an event for that path. Bursts of events on the same path (e.g. an editor's write-then-rename save) are coalesced by a 250ms debounce timer so a single save dispatches once, not several times.
3. Once the debounce fires, the Watcher resolves the file path to a Note ID and calls `Upsert(id)` (or `Remove(id)` for a delete/rename-away) on its `Target` — in practice, `State`.
4. `State.Upsert` takes its single mutex (see ADR-0008 — the Engines themselves stay single-threaded and lock-free by design; `State` is the one place that introduces concurrency safety for the Watcher goroutine and HTTP handler goroutines to share the Engines safely) and calls `Engines.UpsertAll(id)`, which fans out to `Index.Upsert`, `SearchStore.Upsert`, and `Graph.Upsert` in turn. Each is attempted regardless of the others' failure, and their errors are joined — a parse failure in one engine doesn't leave the other two stale for that note.
5. `State.Upsert` then calls `notify()`, pinging every subscribed channel — including on partial engine failure, since an occasional spurious ping is cheaper than a real update going unreflected (see ADR-0009).
6. Every browser tab with an open `/events` connection receives a generic "something changed" SSE ping — no per-note payload, by design (ADR-0009): the payload doesn't say *what* changed, so every view reacts identically by re-fetching its own current content via `htmx.ajax`, rather than each view's JS having to reason about relevance.
7. The browser's HTMX re-fetches the current path (an `HX-Request`-tagged GET), the server re-renders that fragment from the now-updated `State`, and the DOM is swapped in place.

## Persistence and startup

`Index`, `SearchStore`, and `Graph` each support gob-encoded persistence (see ADR-0004: gob was chosen over JSON or SQLite for speed and compactness, accepting opacity and Go-only portability since the cache is disposable by design and never meant to be hand-inspected or shared across languages). `engines.LoadOrBuild` loads all three from their `.cache/*.gob` paths if present, falling back independently to a full `Build()` from the Vault for any engine whose cache file is missing or fails to decode — for example after a change to an entry's shape makes an old cache file undecodable (ADR-0010).

The cache is saved only on graceful shutdown (`SIGINT`/`SIGTERM`, handled in `cmd/main.go`), not on every `Upsert`/`Remove` and not on a periodic timer. Saving on every mutation was rejected because a gob `Save()` writes the whole structure, turning every single note edit into a full-structure disk write; a periodic snapshot was rejected for now as unnecessary added complexity. Graceful-shutdown-only matches the disposable/rebuildable philosophy: a hard kill (`kill -9`, OOM, power loss) loses the cache, but that is a slower next startup, not data loss, since `Build()` fully reconstructs all three engines from the Vault (ADR-0010). On shutdown, `main.go` closes the Watcher first — which waits for any in-flight debounced dispatch to finish — before calling `State.Save`, so the save reflects every change the Watcher had already accepted.

## Package map

- **`vault`** — `VaultProvider`: filesystem-level access to the Vault. Discovers notes (`ListNotes`) and reads raw bytes for notes and assets — no Markdown or frontmatter parsing. Backed by local disk today, swappable for Git/S3/remote later without changing callers.
- **`parser`** — pure `Parse(id, raw) (*Note, error)` function and the `Note` type. No I/O, no `VaultProvider` dependency; fully testable on byte slices.
- **`notes`** — `NoteStore`: the note-level abstraction returning parsed `Note` values, composing a `VaultProvider` with `parser.Parse`.
- **`index`** — `Index`: a disposable, rebuildable projection of Vault metadata (`IndexEntry`: ID, title, tags, path, created) supporting lookup by ID and by tag without re-parsing every note.
- **`search`** — `SearchStore`: a disposable, rebuildable store of normalized full text (title, body, aliases) per note, supporting linear substring search (including mid-word matches).
- **`graph`** — `Graph`: a disposable, rebuildable undirected relationship graph over notes, built from each note's `related` frontmatter field, supporting neighbor lookup, shortest path, and orphan detection.
- **`engines`** — `Engines`: Index, SearchStore, and Graph managed as one unit — constructed together, fanned out to together on every note change (`UpsertAll`/`RemoveAll`), and persisted together (`SaveAll`). Adds no concurrency safety of its own.
- **`state`** — `State`: wraps `Engines` behind a single mutex so the Watcher (writer) and HTTP handlers (readers) can share them safely; also owns the SSE subscriber registry and change notifications.
- **`watcher`** — `Watcher`: monitors the Vault for filesystem changes and drives debounced, incremental `Upsert`/`Remove` calls into its targets so they stay current without a full rebuild or server restart.
- **`server`** — the HTTP layer: routes, HTML rendering (layout, note detail, browse, search UI, graph UI), the JSON API, and the `/events` SSE endpoint. Holds no business logic — it orchestrates `State`, `NoteStore`, and `VaultProvider`.
- **`settings`** — durable `~/.config/ks/settings.json` persistence (active vault path, theme, vault history), loaded at boot and written on every vault switch or theme change. Distinct from the disposable per-vault Engines cache.
- **`activevault`** — `ActiveVault`: the single Vault subsystem (provider, store, `State`, `Watcher`) a running instance currently has open, and the switch orchestration (validate, save-and-discard the outgoing vault, load-or-build the incoming one, kick off a background staleness reconciliation) described in ADR-0011.
- **`logger`** — `slog` setup used across the process.

# 06 — Productivity Experience

Status: open
Blocked by: 05

## Goal

Ship the web renderer: the HTML/HTMX/Alpine.js frontend for browsing, searching, and visualizing the graph, plus a filesystem watcher for live updates.

## Deliverables

**Watcher** (`internal/watcher`)
- Recursive `fsnotify` watch over the Vault root, filtered to `*.md` paths only (assets, dotfiles, editor swap files ignored).
- Per-path debounce (~250ms) to coalesce editor save bursts into a single `Upsert` call.
- Create/Write → `Upsert(id)`. Remove → `Remove(id)`. Rename handled as two independent operations: `Remove(oldID)` on the old path's rename event, `Upsert(newID)` on the new path's create event — no event correlation.
- A mutex owned by the wiring layer (not inside `internal/index`/`internal/search`/`internal/graph`) guards `Index`, `SearchStore`, and `Graph` against concurrent access between the Watcher goroutine and HTTP handlers. See ADR-0008.

**Cache persistence** (`cmd/main.go`)
- On startup: try `Load()` for `Index`/`SearchStore`/`Graph` from `.cache/`; fall back to full `Build()` if missing or corrupt.
- Add graceful shutdown handling (`SIGINT`/`SIGTERM`) that stops the Watcher and `Save()`s all three structures before exit. No periodic snapshot, no save-on-every-`Upsert`. See ADR-0010.

**New engine methods**
- `Index.All() []IndexEntry` — full listing for the browse page.
- `Graph.All() []GraphEntry` — full node/edge dump for graph visualization (no pagination/depth-limiting).

**HTTP: new HTML routes, existing JSON API untouched**
- Existing JSON API (`/health`, `/search`, `/graph/*`) is unchanged and stays the machine-readable contract.
- New routes render HTML/HTMX fragments via stdlib `html/template`, kept separate from the JSON routes (no content negotiation on shared routes):
  - `GET /` — browse/home page, full note list (`Index.All()`), filterable by tag.
  - `GET /notes/{id}` — note detail: `goldmark`-rendered body, graph neighbors, related notes.
  - `GET /search/ui` — HTML search page; HTMX-submits to itself, a fragment-returning results endpoint. (Not bare `/search` — that path is already the JSON API's.)
  - `GET /graph/ui` — Cytoscape.js visualization, fetches `GET /graph/data` (new endpoint backed by `Graph.All()`). (Not bare `/graph` — kept consistent with `/search/ui`'s naming.)
  - `GET /events` — SSE endpoint; the Watcher broadcasts a generic "something changed" ping (no per-note payload) on every `Upsert`/`Remove`. Listening views react by re-fetching their own current content via HTMX. See ADR-0009.

**Frontend** (`web/`)
- HTMX, Alpine.js, Cytoscape.js vendored into `web/` and compiled into the binary via `go:embed` — no CDN, no disk-served assets at runtime.
- Theme: a `data-theme` attribute driven by `theme.default` from config, mapped to a CSS file baked into `web/themes/`. No runtime/user-facing theme switcher in this ticket.

**New dependencies**: `fsnotify`, `goldmark`. See ADR-0007.

## Exit criteria

- Editing a note in the Vault (create, edit, delete, rename) is reflected in the browse list, note detail, search, and graph views without a server restart.
- Search and graph views are navigable in a browser.
- `theme.default` actually changes the rendered theme.
- Existing JSON API endpoints (`/health`, `/search`, `/graph/*`) continue to work unchanged.
- Server starts from a `.cache/` load when present, and persists cache on graceful shutdown.

## Comments

Grilled 2026-07-18 — all open design questions resolved (watcher mechanism, concurrency model, live-update transport, rendering pipeline, asset delivery, theming scope, API/HTML route split, cache persistence lifecycle, rename handling). See ADR-0007 through ADR-0010 and `CONTEXT.md`'s `Watcher` entry. Ready for implementation.

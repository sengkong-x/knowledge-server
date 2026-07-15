# 04 — Search Engine

Status: open
Blocked by: 03

## Goal

Provide full-text and tag-based search over the index built in ticket 03, meeting the < 50ms search performance target from the spec.

## Deliverables

- `internal/search` — defines a `SearchStore` type, built from `NoteStore`, holding normalized (lowercased) title/body/alias text per note. Persisted on disk under `.cache/`, mirroring `Index`'s `Build`/`Upsert`/`Remove`/`Save`/`Load` shape (see ADR-0005).
- Substring query support: matches any contiguous substring, including mid-word (e.g. "lock" matches "Clock") — not token/word-boundary search.
- Results ranked title-match-first (entries matching in `Title` before entries matching only in body/aliases), sorted alphabetically by `Title` within each tier (a deterministic tiebreak — `SearchStore`'s internal storage is a map, so there is no meaningful "original order" to preserve).
- Each result includes a short snippet of surrounding matched text, not just note metadata.
- Tag filtering reuses `internal/index.Index.ByTag` — `SearchStore` does not duplicate tag data. `internal/server` composes both when serving a request.
- `GET /search?q=...&tag=...` endpoint in `internal/server`. AND semantics when both `q` and `tag` are given; `400 Bad Request` when neither is given.

## Exit criteria

- Search returns relevant results for a title/body substring (including mid-word matches) and for a tag match, against a vault fixture.
- Combined `q`+`tag` query returns the intersection; a request with neither returns 400.
- Search latency stays under the spec's 50ms target for a representative fixture size, verified via a `go test -bench` benchmark (not asserted in CI as a hard pass/fail).

## Comments

Draft placeholder — original ticket content was lost; this is a fresh draft from the roadmap description in `spec.md`, not a reconstruction. Grilling pass complete (2026-07-15) — see decisions below, `CONTEXT.md` (`SearchStore`), and ADR-0005.

### Grilling decisions

- **Structure**: `SearchStore` is its own persisted structure (`Build`/`Upsert`/`Remove`/`Save`/`Load`), not a stateless wrapper over `NoteStore` — ready for ticket 06's watcher to call `Upsert`/`Remove` incrementally, same as `Index`.
- **Ranking tiebreak**: "stable" here means *deterministic*, not "preserves original order" — `SearchStore`'s entries are stored in a map, so there is no meaningful pre-sort order to begin with. The tiebreak within a rank tier is alphabetical by `Title`.
- **Incidental footprint**: implementing `SearchStore.Upsert`'s path-resolution mirrored `Index.Upsert`'s identical logic, so both were consolidated onto a shared `vault.ResolvePath`/`vault.RefPath` helper — touching `internal/index/index.go` and `internal/vault/vault.go` beyond this ticket's originally-scoped packages. Also added `IndexEntry.HasTag` (used by both `Index.ByTag` and the composed `/search` handler) and a shared `internal/vaultfixture` test helper package (replacing three near-identical copies of "write a fixture note file" across `internal/index`, `internal/search`, and `internal/server`'s tests). All three are reuse/cleanup, not new behavior.
- **Match semantics**: true substring matching (mid-word included), not word/token search — see ADR-0005 for why this ruled out a plain inverted index.
- **Internal shape**: cached full-text linear scan (`strings.Contains` over precomputed lowercased text), not a trigram index — see ADR-0005. The persisted cache exists to avoid re-parsing notes on every query, not to avoid scanning.
- **Aliases**: folded into the same searchable text blob as title/body — no separate alias-specific lookup API. An alias is a name for finding the note via substring search, unlike a tag.
- **Markdown**: indexed raw (not stripped of Markdown syntax) — stripping is `internal/markdown`'s future territory, not in scope here.
- **Tag filtering**: delegated to existing `Index.ByTag`, not duplicated in `SearchStore`. `internal/server` composes both engines' results — this is orchestration, not business logic, per the spec's HTTP-layer boundary.
- **Endpoint**: single `GET /search?q=...&tag=...`, AND semantics, 400 if neither param given.
- **Response shape**: metadata (`id`, `title`, `path`, `tags`) plus a short matched-text snippet.
- **Performance verification**: a benchmark (`go test -bench`) reports latency against the 50ms target; not a hard-asserted test, to avoid CI flakiness at this scale.

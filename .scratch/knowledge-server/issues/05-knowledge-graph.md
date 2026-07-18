# 05 — Knowledge Graph

Status: done
Blocked by: 04

## Goal

Build the graph engine that surfaces relationships between notes (via the `related` frontmatter field) for graph visualization and navigation.

## Deliverables

- `internal/graph` — builds an undirected note relationship graph from the `related` frontmatter field, resolved by Note ID. Mirrors `internal/index`/`internal/search`'s shape: `Build(provider, store) (*Graph, BuildReport, error)` and `Load(path, provider, store) (*Graph, error)` as free functions; `(*Graph).Save(path) error`, `(*Graph).Upsert(id) error`, `(*Graph).Remove(id)` as pointer methods.
- Query API on `*Graph`:
  - `Neighbors(id string) ([]string, error)` — direct (1-hop) neighbor IDs, sorted. Errors if `id` is not a known node.
  - `ShortestPath(fromID, toID string) ([]string, bool, error)` — unweighted BFS over undirected edges; returns the ordered path (inclusive of both endpoints) and `found=true`, or `(nil, false, nil)` if the notes are known but no path exists. Returns a non-nil error (not `found=false`) if either ID is unknown.
  - `Orphans() []string` — all note IDs with zero edges, sorted.
- `internal/server` endpoints:
  - `GET /graph/neighbors?id=X` → `{"neighbors": [...]}`, 404 if `id` unknown.
  - `GET /graph/path?from=X&to=Y` → `{"path": [...], "found": bool}`, 404 if either ID unknown.
  - `GET /graph/orphans` → `{"orphans": [...]}`.
- Cache under `.cache/`, gob-encoded like `Index`/`SearchStore` (ADR-0004), rebuildable from the Vault.

## Explicitly out of scope

- **In-body link detection** (e.g. wikilinks, Markdown links in `Note.Body`). Requires `internal/markdown` (not yet built; `goldmark` isn't in `go.mod`). Deferred to a follow-up ticket once that dependency exists.
- **Aliases as edge source.** `aliases` frontmatter creates no graph edges — only `related` does. Aliases may still appear as display metadata on a `GraphEntry` but never affect connectivity.
- **Directed edges / backlink distinction.** See ADR-0006 — edges are undirected; there's no "who declared this link" tracking.

## Domain decisions (see CONTEXT.md, ADR-0006)

- `related` entries resolve by Note **ID** only (not by alias or title).
- Edges are **undirected/symmetric**: `related: [B]` on A creates a shared edge, even if B doesn't list A back.
- **Dangling references** (a `related` ID that doesn't exist in the Vault) are silently dropped when building edges — no phantom node is created — and reported in the graph's `BuildReport`, mirroring `index.BuildReport.Failed`.
- **Self-references and duplicates** in a note's `related` list are silently normalized (no self-loops, no multi-edges); not reported as warnings.
- **Upsert semantics**: `Upsert(id)` re-parses `id` and replaces its outgoing edge set; because edges are shared records, the other side of each edge updates automatically. A previously-dangling reference that becomes resolvable (its target is created/upserted later) resolves lazily the next time that edge is read, not via a forced cross-note recomputation.

## Exit criteria

- Graph correctly links notes via `related` frontmatter (by ID) on a vault fixture, including the case where only one side declares the relation.
- Dangling `related` references are dropped and reported, not treated as fatal build errors.
- Orphaned notes (zero edges after symmetrizing) are queryable via `Orphans()` and `GET /graph/orphans`.
- `Neighbors` and `ShortestPath` behave correctly for: direct neighbors, multi-hop shortest path, disconnected-but-known notes (`found=false`), and unknown note IDs (error / 404).

## Comments

Draft placeholder — original ticket content was lost; a first draft was reconstructed fresh from the roadmap description in `spec.md`, then put through a grilling session (2026-07-18) to resolve scope and semantics before implementation. Key resolutions: in-body links and aliases descoped as edge sources; undirected graph model recorded in ADR-0006; dangling-reference and self-reference/duplicate handling defined; persistence and server API shaped to mirror `internal/index`/`internal/search` conventions.

Implemented 2026-07-18 via TDD (11 vertical slices, one seam per test): `internal/graph` (Build/Load/Save/Upsert/Remove/Neighbors/ShortestPath/Orphans) and the three `/graph/*` server endpoints. `ShortestPath`'s final signature is `([]string, bool, error)` — the error return for unknown IDs was added during implementation and wasn't reflected in the original draft signature above (now corrected). All package and server tests pass; `cmd/main.go` wired to build the graph alongside the index and search store at startup.

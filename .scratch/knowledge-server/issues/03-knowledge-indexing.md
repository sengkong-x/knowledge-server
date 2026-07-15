# 03 — Knowledge Indexing

Status: open
Blocked by: 02

## Goal

Build the indexing engine that scans the Vault (via `NoteStore`) and maintains a disposable, rebuildable `Index` of notes and their metadata.

## Deliverables

- `internal/index` — defines `IndexEntry` (ID, Title, Tags, Path, Created — no Body) and an `Index` type built from a `NoteStore`, persisted on disk under `.cache/` using gob encoding (see ADR-0004).
- `Index.ByID(id)` / `Index.ByTag(tag)` lookups returning `IndexEntry` values.
- `Index.Upsert(id)` / `Index.Remove(id)` — real methods, exercised by this ticket's own tests, even though nothing calls them from a watcher yet (that's ticket 06+).
- Rebuild-from-scratch support — deleting the on-disk index must never lose knowledge, only rebuild cost.
- Notes that fail to parse during a build are skipped and recorded in a build report; one bad note must not fail the whole build.

## Exit criteria

- Index can be built from a vault fixture and queried by ID and by tag.
- Deleting the on-disk index and rebuilding produces a semantically identical result (same entries and field values; byte-for-byte identity is not required).
- A vault fixture containing one note with invalid frontmatter still produces a complete index for every other note, with the bad note's ID surfaced in the build report.
- `Index.Upsert` and `Index.Remove` are covered by tests that add/remove a note and re-query.

## Comments

Draft placeholder — original ticket content was lost; this is a fresh draft from the roadmap description in `spec.md`, not a reconstruction. Grilling pass complete (2026-07-15) — see decisions below and `CONTEXT.md` (`Index`, `IndexEntry`) and ADR-0004.

### Grilling decisions

- **Storage**: on-disk under `.cache/`, not in-memory-only — makes "delete and rebuild" a literal, testable operation.
- **Query surface**: new `Index` type in `internal/index`, not an extension of `NoteStore` — keeps the parse/index boundary ADR-0003 already drew.
- **Bad notes**: skipped and recorded, don't abort the build.
- **Rebuild identity**: semantically identical, not byte-identical — no requirement for deterministic encoding.
- **Entry type**: new `IndexEntry`, distinct from `NoteRef` (pre-parse, ID+path only).
- **Incremental hook**: `Upsert`/`Remove` shipped and tested now, not deferred as an untested "hook point."
- **On-disk format**: gob encoding — see ADR-0004.
- **Provider/store ownership**: `Index` holds its `VaultProvider`/`NoteStore` internally (set by `Build` or `Load`), so `Upsert(id)`/`Remove(id)` take only an ID — matching this ticket's stated deliverable — rather than requiring callers to re-supply both on every call.

# 03 — Knowledge Indexing

Status: open
Blocked by: 02

## Goal

Build the indexing engine that scans the Vault (via `NoteStore`) and maintains a disposable, rebuildable index of notes and their metadata.

## Deliverables

- `internal/index` — builds and maintains an in-memory (or on-disk, under `.cache/`) index of notes: ID, title, tags, path, timestamps.
- Rebuild-from-scratch support — deleting the index must never lose knowledge, only rebuild cost.
- Incremental update hook point for a future filesystem watcher (ticket 06+).

## Exit criteria

- Index can be built from a vault fixture and queried by ID/tag.
- Deleting the on-disk index and rebuilding produces an identical result.

## Comments

Draft placeholder — original ticket content was lost; this is a fresh draft from the roadmap description in `spec.md`, not a reconstruction. Needs a grilling pass before implementation starts.

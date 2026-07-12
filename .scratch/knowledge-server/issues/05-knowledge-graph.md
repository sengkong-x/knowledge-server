# 05 — Knowledge Graph

Status: open
Blocked by: 04

## Goal

Build the graph engine that surfaces relationships between notes (via the `related`/`aliases` frontmatter and detected links) for graph visualization and navigation.

## Deliverables

- `internal/graph` — builds a note relationship graph from frontmatter (`related`) and in-body links/aliases.
- Graph query API (neighbors of a note, shortest path, orphan detection) exposed through `internal/server`.
- Cache under `.cache/`, rebuildable from the Vault like the search index.

## Exit criteria

- Graph correctly links notes via `related` frontmatter and aliases on a vault fixture.
- Orphaned notes (no incoming/outgoing links) are queryable.

## Comments

Draft placeholder — original ticket content was lost; this is a fresh draft from the roadmap description in `spec.md`, not a reconstruction. Needs a grilling pass before implementation starts.

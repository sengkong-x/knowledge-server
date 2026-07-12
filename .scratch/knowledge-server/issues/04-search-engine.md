# 04 — Search Engine

Status: open
Blocked by: 03

## Goal

Provide full-text and tag-based search over the index built in ticket 03, meeting the < 50ms search performance target from the spec.

## Deliverables

- `internal/search` — full-text search over note bodies/titles, plus tag/alias filtering.
- HTTP endpoint(s) exposing search, wired through `internal/server`.

## Exit criteria

- Search returns relevant results for a title/body substring and for a tag match, against a vault fixture.
- Search latency stays under the spec's 50ms target for a representative fixture size.

## Comments

Draft placeholder — original ticket content was lost; this is a fresh draft from the roadmap description in `spec.md`, not a reconstruction. Needs a grilling pass before implementation starts.

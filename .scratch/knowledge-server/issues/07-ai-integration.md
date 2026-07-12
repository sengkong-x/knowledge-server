# 07 — AI Integration

Status: open
Blocked by: 06

## Goal

Expose an AI Context API so agents can understand context, update notes, suggest relationships, and generate learning materials, per the spec's success criteria.

## Deliverables

- `internal/api` — AI-facing API: fetch note + related context, propose edits, suggest relationships from the graph.
- Write path back to the Vault (creating/editing notes) with the same fail-fast/rebuildable guarantees as the read path.

## Exit criteria

- An agent can retrieve a note plus its graph neighbors as context in one call.
- An agent can create or edit a note through the API and see it reflected via `NoteStore`/index/graph without manual intervention.

## Comments

Draft placeholder — original ticket content was lost; this is a fresh draft from the roadmap description in `spec.md`, not a reconstruction. Needs a grilling pass before implementation starts.

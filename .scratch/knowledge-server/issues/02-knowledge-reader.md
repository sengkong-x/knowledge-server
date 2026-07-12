# 02 — Knowledge Reader

Status: open
Blocked by: 01

## Goal

Implement the Markdown/YAML parser and the `NoteStore` interface declared in ticket 01, turning raw Vault bytes (via `VaultProvider`) into parsed `Note` values.

## Deliverables

- `internal/parser` — Markdown + YAML frontmatter parsing into `Note` (title, tags, aliases, related, status, created, body).
- `internal/parser` (or a new `internal/notes`) — `NoteStore` implementation backed by `VaultProvider`, satisfying the interface from ticket 01.
- Error handling for malformed frontmatter / missing required fields.

## Exit criteria

- `NoteStore.List()` and `NoteStore.Load(id)` return parsed notes from a real vault fixture.
- Malformed notes produce a clear error rather than a panic or silent skip.

## Comments

Draft placeholder — original ticket content was lost (see `.scratch/` recovery note in project history) and this is a fresh draft from the roadmap description in `spec.md`, not a reconstruction. Needs a grilling pass before implementation starts.

# 02 — Knowledge Reader

Status: done
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

Implemented — grilling session resolved the open questions the draft left implicit:

- `internal/parser` stays pure (`Parse(id string, raw []byte) (*Note, error)`, no I/O); `internal/notes` holds `NoteStore` and its `VaultProvider`-backed implementation (see ADR-0003).
- `Note` gained `Tags`, `Aliases`, `Related []string`, `Status string`, `Created time.Time` alongside the existing `ID`, `Title`, `Body`.
- `title` and `created` are required; `tags`/`aliases`/`related`/`status` are optional. A note missing frontmatter entirely fails the same "missing required field" check as any other note missing `title` — no separate error path.
- `status` accepts either a YAML scalar or a single-element list; unknown frontmatter fields are silently ignored (not strict-decoded).
- `created` parses into `time.Time` via `yaml.v3`'s native timestamp support (bare date or full RFC3339).
- Frontmatter is split from the body by hand (no frontmatter library), then decoded with the existing `gopkg.in/yaml.v3` dependency. Body is `strings.TrimSpace`'d.
- `NoteStore.List()` fails fast on the first malformed note. `NoteStore.Load(id)` returns a sentinel `notes.ErrNotFound` (`errors.Is`-checkable) for missing IDs, distinct from parse errors.
- Tests colocated per package (`internal/parser/parser_test.go`, `internal/notes/notes_test.go`), following the existing `t.TempDir()` + `writeFile` fixture pattern from `internal/vault/vault_test.go`.

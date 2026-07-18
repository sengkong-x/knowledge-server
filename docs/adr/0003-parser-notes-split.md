---
title: Split internal/parser (pure parsing) from internal/notes (NoteStore)
created: 2026-07-14
tags: [adr]
---

Ticket 01 declared `NoteStore` inside `internal/parser` only because the parser didn't exist yet — that was never meant to be its permanent home. For ticket 02, we split parsing from storage into two packages: `internal/parser` holds `Note` and a pure `Parse(id string, raw []byte) (*Note, error)` function with no I/O and no `VaultProvider` dependency, fully testable on byte slices. `internal/notes` holds the `NoteStore` interface and its `VaultProvider`-backed implementation, composing `vault.VaultProvider` + `parser.Parse`. This keeps `internal/notes` depending on `internal/parser` (never the reverse), avoids an import cycle, and keeps the parser unit-testable without disk fixtures.

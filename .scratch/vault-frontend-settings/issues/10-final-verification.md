---
title: "Final verification pass"
created: 2026-07-19
tags: [issue]
---

Status: open
Type: task

## Context

Run last, once Tickets 01-09 are all done. This is the "prove it all works together" gate, not a place to introduce new design decisions — if something's wrong, file it back against the relevant earlier ticket rather than improvising a fix here.

## Checklist

1. `go build ./...` and `go vet ./...` clean, zero references to `internal/config` anywhere in `.go` files (`grep -rln "internal/config" --include=*.go .` empty).
2. `go test -race ./...` passes in full, including the new `internal/vault` (canonicalization), `internal/index`/`internal/search`/`internal/graph` (versioning), `internal/settings`, and `internal/activevault` test suites from Tickets 01-05.
3. `config.yaml`, `docs/config.yaml`, and the `--config` flag are gone (`grep -rn "\-\-config\|configPath" cmd/ internal/` empty; `ls config.yaml docs/config.yaml` both fail).
4. End-to-end manual run: fresh clone, `go build -o ks ./cmd`, `./ks --port 8080` with no prior `~/.config/ks/settings.json` — confirm it boots with no Active Vault, the picker is visibly the primary UI, adding a vault path works, switching between two different vaults preserves each one's data (edit a note in vault A, switch to B, switch back to A, confirm the edit persisted and shows without a full rebuild being visibly triggered), theme toggle works and survives a process restart, an invalid vault path shows a visible inline error without disturbing the currently-active vault.
5. Confirm the SSE live-update edge case flagged in Ticket 06 (a connection open across a vault switch) was actually resolved one way or the other, not left as a silent gap — check the ticket's resolution and verify it behaves as documented.
6. Visual pass across both themes per Ticket 08's checklist, plus confirm no `package.json`/`node_modules`/bundler config exists anywhere in the repo.
7. README instructions followed literally, start to finish, on a clean checkout.
8. Re-read `docs/adr/0011-frontend-driven-vault-selection.md` and the 2026-07-19 update to `docs/adr/0010-cache-persistence-lifecycle.md` one more time against the actual final code — flag (don't silently fix) any place the implementation diverged from the documented decision, since the ADR is supposed to be the record of *why*, and a silent divergence would make it actively misleading to a future reader.

## Anti-pattern guards

- Don't treat "tests pass" as sufficient on its own for the frontend tickets (07/08) — those need the manual browser pass in item 4/6, since automated tests won't catch a nav bar that disappears after an HTMX swap or a color-contrast problem.
- Don't quietly patch an ADR to match the code if they've diverged — surface the divergence to the user/reviewer first; the ADR is the record of an intentional decision, and if the code took a different path for a good reason, that's worth a fresh ADR update, not a silent rewrite.

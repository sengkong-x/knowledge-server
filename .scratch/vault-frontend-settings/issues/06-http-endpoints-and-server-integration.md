---
title: "Wire ActiveVault into server.go; add settings/switch-vault/switch-theme endpoints"
created: 2026-07-19
tags: [issue]
---

Status: resolved
Type: task

## Context

Depends on Ticket 05's `internal/activevault.ActiveVault` existing with the API described there. Read `internal/server/server.go` in full before changing it — today `server.New(vaultPath string, provider vault.VaultProvider, store notes.NoteStore, s *state.State, theme string) http.Handler` (server.go:154) captures all five params as **immutable** closure values across ~14 route handlers (full route table in the spec's Phase 0 section). This ticket changes that signature and touches every handler that currently reads `vaultPath`, `provider`, `store`, `s`, or `theme` directly.

## What to implement

1. Change `server.New`'s signature to `func New(av *activevault.ActiveVault) http.Handler`.
2. In every existing route handler that used `vaultPath`/`provider`/`store`/`s`/`theme` directly (the full list, per Phase 0 discovery: lines ~166, 184, 209, 212-213, 216-244 (`/events`, uses `s.Subscribe()`), 246, 263-295, 298-310 (`/health`, uses `vaultPath`), 307 (`/search`, uses `s`), 356, 366, 379, 384), replace the direct closure read with `path, provider, store, s, ok := av.Snapshot()` (plus `av.Theme()` where `theme` was used) at the top of the handler, and handle `ok == false` (no Active Vault selected) by rendering a clear "no vault selected — pick one" state instead of the normal content — **do not** let handlers panic or 500 on a nil `provider`/`store`/`s` when nothing is selected; this is now a first-class, expected state (Ticket 04 established the server boots into it).
3. `layoutTemplate` (server.go:25-43) currently gets `Theme` from a closure captured once at `New()` time — change `layoutView` population (server.go:45-49 area) to call `av.Theme()` fresh on every render instead, so a theme switch (Ticket 05's `SetTheme`) takes effect on the very next page render with zero extra plumbing (no need to touch the SSE mechanism for this — a normal page load already re-executes the template).
4. New endpoints (pick REST-ish paths consistent with the existing table's style — plain nouns, no versioning prefix, matching e.g. `/graph/data`, `/graph/neighbors`):
   - `GET /settings` — JSON: current `vault_path` (or empty), `theme`, `vault_history` (read via `settings.Load()`, Ticket 03 — note `ActiveVault` itself doesn't hold history, only the currently-active path; history lives in settings.json).
   - `PUT /vault` (or `POST /vault/switch` — pick one, be consistent with whichever verb convention feels closer to the existing table; there's no strong existing precedent since all current routes are `GET`, so document the choice in a comment) — body `{"path": "..."}`. Calls `av.Switch(path)`; on success, `settings.Save(settings.Load().WithVault(canonicalPath))`, respond 200; on validation failure, respond 4xx with the error message so Ticket 07's picker UI can display it inline (per the grill's Q9-adjacent decision that switch failures must be visible in the UI, not silently swallowed).
   - `PUT /theme` (or matching verb choice above) — body `{"theme": "..."}`. Calls `av.SetTheme(theme)`, then `settings.Save(settings.Load().WithTheme(theme))`, respond 200.
5. Update `cmd/main.go` (from Ticket 04's boot sequence) to construct `av := activevault.New(loadedSettingsTheme)`, attempt `av.Switch(settings.VaultPath)` at boot if one was persisted (log+continue on failure, per Ticket 04), then `server.New(av)`.

## Verification checklist

- `go build ./...` succeeds.
- Existing route handlers still return correct content when a vault *is* selected — run the existing `internal/server` test suite (check `internal/server/server_test.go` if present, or add coverage if none exists) against a fresh `ActiveVault` that's had `Switch` called on it in the test setup.
- New test: hitting any content route (e.g. `/notes/{id}`, `/search`, `/graph/data`) with **no** Active Vault selected returns a sensible response (a rendered "no vault selected" page for HTML routes, a clear error JSON for API routes) — not a panic, not a 500 with a nil-pointer stack trace.
- New tests for `GET /settings`, `PUT /vault` (success + validation-failure cases), `PUT /theme`.
- Manually verify (or write an integration test) that switching vaults via `PUT /vault` causes a subsequent page load to reflect the new vault's content, and that the `/events` SSE stream still delivers pings correctly against the *new* `State` after a switch (this is the part most likely to have a subtle bug — the SSE handler's `s.Subscribe()` call happens per-connection at request time via `Snapshot()`, so an existing long-lived SSE connection from *before* a switch will still be subscribed to the *old* `State`, which is now discarded; decide whether to explicitly close old SSE connections on switch or accept they'll just stop receiving pings until the client reconnects — document whichever you pick, don't leave it as an unnoticed gap).

## Anti-pattern guards

- Don't leave any handler still closing over the vault-path/provider/store/state/theme values from a one-time `New()` call — that's exactly the bug this ticket exists to fix (stale references after a switch).
- Don't let a missing Active Vault cause a nil-pointer panic anywhere in the route table — every handler must check `ok` from `Snapshot()`.
- Don't forget the SSE-connections-across-a-switch edge case called out above — it's easy to miss since it won't show up in a simple manual test (you'd need a live `/events` connection open during a switch to notice it).

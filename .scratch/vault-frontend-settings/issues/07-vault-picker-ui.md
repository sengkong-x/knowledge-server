---
title: "Frontend vault picker: history dropdown + add-new-vault form"
created: 2026-07-19
tags: [issue]
---

Status: open
Type: task

## Context

Depends on Ticket 06's `GET /settings`, `PUT /vault`, `PUT /theme` endpoints existing. Read `internal/server/server.go`'s `layoutTemplate` (lines 25-43) and the existing HTMX-based pattern in `searchUITemplate` (lines 99-104, `<form hx-get="/search/ui" hx-target="body">`) before writing new markup — match that existing HTMX idiom rather than introducing a different frontend pattern (e.g. don't add fetch()/JS-driven form handling if `hx-put`/`hx-target` already does the job, since htmx and Alpine are already vendored and in use; see `web/vendor/htmx.min.js`, `web/vendor/alpine.min.js`).

There is currently **no nav bar anywhere** in the layout (confirmed: no `<nav>` in `layoutTemplate`) — this ticket is where one gets added for the first time, not a restyle of an existing one.

## What to implement

1. Add a small nav/header region to `layoutTemplate` (server.go:25-43) containing the vault picker, rendered on every page (it's global chrome, like the theme CSS links already are).
2. Vault picker markup: an Alpine-driven dropdown (`x-data`, given Alpine's already vendored) listing `vault_history` from `GET /settings`, each entry an `hx-put="/vault"` (or whichever verb Ticket 06 chose) trigger with `hx-vals` carrying that path, plus an "Add new vault..." toggle that reveals a text `<input>` + submit wired the same way for an arbitrary new path.
3. On successful switch, rely on the existing SSE-driven refresh (`layoutTemplate`'s inline `EventSource`/`htmx.ajax` re-fetch, server.go:37-41) — but note Ticket 06 flagged that a switch discards the old `State`, so the *current* page's own SSE connection may need an explicit refresh rather than waiting on a ping that'll never come from the now-defunct old State. Simplest correct approach: have the `PUT /vault` response itself trigger a full page reload/re-render client-side (e.g. `hx-target="body"` on the switch request itself, swapping in the freshly rendered page for the new vault) rather than depending on the SSE ping for this specific transition.
4. Surface `PUT /vault` validation failures (Ticket 06: 4xx + error message body) inline near the picker — e.g. an HTMX error-target swap showing the message, not a browser alert or a silently-ignored failed request.
5. Add a theme toggle (light/dark) in the same nav region, wired to `PUT /theme` the same way — this is the natural place for it since Ticket 06 made theme frontend-switchable.
6. When no Active Vault is selected (Ticket 06's `ok == false` path), the picker itself IS the empty-state's primary call to action — don't build a separate "no vault" page that's disconnected from the picker; the picker should be prominent/expanded by default in that state.

## Verification checklist

- Manually run the server (`go run ./cmd -port 8080`), confirm: fresh start with no settings.json shows the empty state with the picker prominent; adding a new vault path switches successfully and the page reflects vault content; switching back to a previously-added vault via the dropdown works; an invalid path shows a visible inline error and does *not* lose the currently-active vault.
- Confirm theme toggle changes `data-theme` on the next render and persists across a restart (`settings.json`'s `theme` field, per Ticket 03/06).
- No regressions to existing pages (Browse `/{$}`, Search `/search/ui`, Graph `/graph/ui`, note detail `/notes/{id}`) — the nav addition shouldn't break their existing HTMX targets (`hx-target="body"` swaps currently replace the whole `<body>`, so the nav must either survive that swap correctly or the swap target needs narrowing — check this carefully, it's an easy place for the nav to disappear after the first HTMX navigation).

## Anti-pattern guards

- Don't introduce a JS framework or bundler for this — Alpine + htmx (already vendored) are sufficient for a dropdown + form + toggle; per Ticket 08/the spec, zero-build-step is a hard constraint.
- Don't build a custom SSE reconnection scheme — reuse the existing `/events` mechanism for ongoing live-updates; only the vault-switch action itself needs special handling (per point 3 above) because it's a request the *user* initiated, not a background change.
- Don't forget the `hx-target="body"` nav-survival issue flagged in the verification checklist — it's the single most likely regression from adding chrome to a layout that previously had none.

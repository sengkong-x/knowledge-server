---
title: Frontend-driven vault selection, no static config file
created: 2026-07-19
tags: [adr]
---

`config.yaml` held three values: `server.port`, `vault.path`, `theme.default`. We removed it entirely. `server.port` becomes a `--port` CLI flag (a value only ever needed once, at process start). `vault.path` and `theme` become runtime-selectable from the frontend instead: the server now boots with no vault selected, and the user picks one (or the last-used one is auto-restored) through the UI. We considered keeping vault_path as a required startup argument with the frontend only able to override it afterward, but rejected that in favor of frontend-only selection — it better matches the actual usage pattern (a personal tool where "which vault am I looking at" is a browsing decision, not a deployment decision) and removes the awkwardness of a server that's simultaneously configured by flag and by UI.

Because a running instance can now switch between several vaults over its lifetime, the per-vault engine cache (`Index`/`SearchStore`/`Graph`, gob-encoded per ADR-0004) moved from a single directory colocated with `config.yaml` to `~/.cache/ks/<hash>/`, where `<hash>` is derived from the canonicalized (tilde-expanded, absolute, symlink-resolved) vault path. This makes the cache location reproducible per vault regardless of how the path is written, and lets switching back to a previously-visited vault reuse its cache instead of rebuilding.

The "current selection" (vault_path + theme) is separate from that disposable cache: it's the one piece of state that must survive a `~/.cache` wipe, so it lives in `~/.cache/ks/<hash>/`'s sibling, `~/.config/ks/settings.json`, alongside a small history list of previously-opened vault paths (for the picker UI). Theme is deliberately global (one value shared by all clients of the instance), not a per-browser preference, matching how it already works today (baked server-side into every rendered page).

## Consequences

Running two vaults concurrently in separate processes (e.g. the main vault on one port and `docs/` served as a second vault on another, as before) is no longer supported — `settings.json` holds one "current vault" per machine, and two processes would fight over it. Switching between vaults is now done within a single running instance via the picker instead. If concurrent multi-instance access becomes a real need, `settings.json`'s path would need to become instance-scoped (e.g. keyed by `--port`), which this ADR does not do.

## Addendum (2026-07-20): removing history entries, and a directory browser for adding one

Two gaps in the original picker: entries in `VaultHistory` could only ever be switched to, never removed, and "adding" a vault meant typing an absolute path into a free-text input — awkward for a personal tool where users generally don't know the exact path by heart.

**Removal.** Each history row now carries a remove control (`DELETE /vault`, `settings.Settings.WithoutVault`). Removing a path drops it from `VaultHistory` and deletes its disposable `~/.cache/ks/<hash>/` (`ActiveVault.RemoveVault`) — cheap to rebuild, so no confirmation prompt guards this. Removing the *currently active* vault is allowed and clears the active selection entirely (`VaultPath` reset to empty, in-memory state torn down) rather than falling back to the next-most-recent history entry — the same "no vault selected" state the server already boots into, kept as the single fallback path instead of adding a second one.

**Adding a vault.** The free-text input is replaced by a directory-browser modal (`vaultBrowser()` in `web/js/vault-browser.js`) that walks the filesystem one level at a time via a new `GET /vault/browse?path=` endpoint (`internal/vault.ListDirectories` — a plain, non-recursive `os.ReadDir`, distinct from `ListNotes`'s recursive markdown walk). This exists because a browser can never hand the server an absolute OS path from a native picker (`<input type=file>`/File System Access API both withhold real paths, by design, for sandboxing) — server-side listing is the only way to browse-then-select a real filesystem path from a web UI here. The browser starts at `$HOME`, hides dotfolders, and shows the current path as an editable breadcrumb (click to type/paste and jump directly) alongside a persistent "Use this folder" button that submits to the existing `PUT /vault` switch flow. `vault.ValidateRoot`'s already-permissive rule (any stat-able directory, no marker file) is unchanged — the browser just gives users a way to point at one without typing it.

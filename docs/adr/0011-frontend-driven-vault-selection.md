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

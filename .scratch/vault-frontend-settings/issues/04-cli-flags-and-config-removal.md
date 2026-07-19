---
title: "Replace --config with --port; delete internal/config; boot with no Active Vault"
created: 2026-07-19
tags: [issue]
---

Status: resolved
Type: task

## Context

This ticket touches `cmd/main.go`'s wiring directly. Read the full current file first — the exact wiring sequence today is:

```
1. flag.String("config", "./config.yaml", ...); flag.Parse()          — main.go:24-25
2. log := logger.New()                                                 — :27
3. cfg, err := config.Load(*configPath)                                 — :29
4. vault.ValidateRoot(cfg.Vault.Path)                                   — :35
5. provider := vault.NewLocalVaultProvider(cfg.Vault.Path)              — :40
6. store := notes.NewVaultNoteStore(provider)                          — :41
7. cacheDir := filepath.Join(filepath.Dir(*configPath), ".cache"); MkdirAll  — :47-51
8. cachePaths := engines.Paths{Index: "index.gob", Search: "search.gob", Graph: "graph.gob"}  — :52-56
9. e, report, err := engines.LoadOrBuild(cachePaths, provider, store)   — :58
10. knowledge := state.New(e)                                          — :73
11. w, err := watcher.New(cfg.Vault.Path, knowledge)                   — :75
12. w.Start()                                                          — :80
13. handler := server.New(cfg.Vault.Path, provider, store, knowledge, cfg.Theme.Default)  — :85
14. httpServer := &http.Server{Addr: addr, Handler: handler}           — :88
15. signal.NotifyContext(..., SIGINT, SIGTERM); go httpServer.ListenAndServe(); <-ctx.Done()  — :90-101
16. shutdown: w.Close() → knowledge.Save(cachePaths) → httpServer.Shutdown(...)  — :104-113
```

Steps 3-13 all depend on having a vault path up front, which no longer exists at boot (ADR-0011: no Active Vault until the frontend picks one). This ticket does **not** implement the full vault-switch orchestration (that's Ticket 05) — it establishes the new boot sequence and flag surface, delegating "what happens when a vault gets selected" to whatever Ticket 05 produces. If Ticket 05 isn't done yet, stub the dependency (e.g. a `TODO(ticket-05)` and a manager type with a minimal placeholder) rather than blocking this ticket on it — but coordinate: in practice, do Tickets 01-04 first (they're independent), then 05 last before 06.

## What to implement

1. Delete `internal/config/` entirely (the package, `config.go`, `config_test.go`).
2. Delete `config.yaml` (repo root, gitignored) and `docs/config.yaml` — the docs-as-second-vault trick these enabled is superseded by the vault picker (ADR-0011 Consequences section: concurrent multi-instance is explicitly dropped).
3. In `cmd/main.go`:
   - Replace the `--config` flag with `--port` (`flag.Int("port", 8080, "HTTP port to listen on")`).
   - Remove steps 3-6, 8, 11-13 of the sequence above from top-level `main()` — they now only happen inside whatever "select/switch Active Vault" operation Ticket 05 exposes, triggered by (a) `settings.Load()` finding a persisted `vault_path` at boot, or (b) a later HTTP request from the picker (Ticket 06).
   - New boot sequence: parse `--port` → `logger.New()` → `settings.Load()` (Ticket 03) → construct whatever manager Ticket 05 defines → if `settings.VaultPath != ""`, attempt to select it at boot (log a warning and continue with no Active Vault if that fails — e.g. the directory was deleted since last run — don't crash the whole server over a stale settings entry) → build the `http.Handler` → run `http.Server` on `:<port>` → on shutdown, save whatever vault is currently active (extends step 16, now routed through Ticket 05's save-on-switch-or-shutdown logic rather than a single hardcoded `cachePaths`).
4. Remove the `"github.com/sengkong/knowledge-server/internal/config"` import once the package is deleted; add whatever new imports Tickets 03/05 introduce.

## Verification checklist

- `internal/config` no longer exists in the repo (`ls internal/config` fails).
- `config.yaml` and `docs/config.yaml` are deleted; `.gitignore:2`'s `/config.yaml` line should also be removed (nothing left to ignore).
- `go build ./...` succeeds with zero references to `internal/config`.
- `grep -rn "internal/config" .` returns nothing outside of `.scratch/` planning docs and ADRs (which reference it historically — that's fine, don't edit ADR prose).
- Starting the binary with `--port 9000` and no persisted settings boots successfully and listens on `:9000` with no Active Vault (verify via whatever "no vault selected" HTTP behavior Ticket 06 defines, or at minimum: process doesn't crash, `/health`-equivalent doesn't panic).

## Anti-pattern guards

- Don't leave a `--config` flag around "for backwards compatibility" — the whole point is removal, per the grill and ADR-0011.
- Don't hardcode a fallback vault path (e.g. `~/knowledge`) if settings.json is empty — Q2 of the grill explicitly chose "no vault selected" over "defaulted startup flag." Booting with nothing selected is the correct, intended behavior, not a gap to paper over.

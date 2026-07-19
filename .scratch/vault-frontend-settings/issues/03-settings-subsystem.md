---
title: "Settings subsystem: ~/.config/ks/settings.json"
created: 2026-07-19
tags: [issue]
---

Status: open
Type: task

## Context

Read `docs/adr/0011-frontend-driven-vault-selection.md` and the **Settings** / **Active Vault** entries in `CONTEXT.md` first — use that exact terminology in doc comments and identifier names (e.g. don't call this "Config" anywhere; that word is reserved for the deleted `config.yaml` concept per the glossary's `_Avoid_` line).

This is new code — no settings-persistence code exists anywhere in the repo today (confirmed by grep).

## What to implement

New package `internal/settings`:

```go
type Settings struct {
    VaultPath    string   `json:"vault_path"`
    Theme        string   `json:"theme"`
    VaultHistory []string `json:"vault_history"`
}

// Path returns ~/.config/ks/settings.json, resolved via os.UserConfigDir()
// (or os.UserHomeDir() + ".config/ks" if UserConfigDir isn't suitable —
// check what os.UserConfigDir() actually returns on Linux and confirm it's
// ~/.config before relying on it; document the choice in a comment).
func Path() (string, error)

// Load reads settings.json. If the file doesn't exist, returns a zero-value
// Settings and no error (first-run: no vault selected yet, no theme, no
// history) — this is a normal, expected case, not a failure.
func Load() (Settings, error)

// Save writes settings atomically (write to a temp file in the same
// directory, then os.Rename) so a crash mid-write can't corrupt it —
// this file is durable state per ADR-0011/CONTEXT.md's Settings entry,
// unlike the disposable gob cache.
func Save(s Settings) error

// WithVault returns a copy of s with VaultPath set and pushed to the front
// of VaultHistory (deduplicated, most-recent-first, capped at a reasonable
// length — e.g. 10 entries).
func (s Settings) WithVault(path string) Settings

// WithTheme returns a copy of s with Theme set.
func (s Settings) WithTheme(theme string) Settings
```

Default theme when `Settings.Theme` is empty: match the existing fallback already in `internal/server/server.go:155-157` (defaults to `"light"`) — don't introduce a second, different default; either keep that fallback where it is, or move the default into this package and document that server.go's existing fallback becomes redundant (pick one, don't have both silently disagree).

`os.MkdirAll` the parent directory (`~/.config/ks/`) before writing, mirroring how `cmd/main.go:48` already does `os.MkdirAll` for the cache dir.

## Verification checklist

- New tests in `internal/settings/settings_test.go`. Use `t.Setenv` to point `HOME`/`XDG_CONFIG_HOME` at a temp dir for isolation (check what `os.UserConfigDir()` actually reads on Linux — likely `$XDG_CONFIG_HOME` or `$HOME/.config` — and set the right env var in tests).
- Cover: `Load()` on a missing file returns zero-value + nil error; `Save()` then `Load()` round-trips exactly; `WithVault` dedupes an already-present path (moves it to front, doesn't duplicate) and caps history length; atomic write (a `Save()` that's interrupted — simulate by checking the temp-file-then-rename pattern exists, not necessarily by fault-injecting an actual crash).
- `go test ./internal/settings/...` passes.

## Anti-pattern guards

- Don't put this under `~/.cache` — ADR-0011 is explicit that Settings is durable state, distinct from the disposable per-vault Engines cache (Ticket 01/02's territory), specifically so a `rm -rf ~/.cache` doesn't silently forget the user's vault.
- Don't reintroduce YAML — this is JSON per the spec; config.yaml is being removed, not renamed.
- Don't validate the vault path here (e.g. don't call `vault.ValidateRoot` inside this package) — Settings is a dumb persistence layer; validation belongs to the vault-switch orchestration in Ticket 05, which is the actual decision point for "is this a real vault."

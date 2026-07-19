---
title: "Update README for --port + vault-picker flow; remove config.yaml references"
created: 2026-07-19
tags: [issue]
---

Status: resolved
Type: task

## Context

Can run any time after Ticket 04 lands (needs the real `--port` flag and boot behavior to describe accurately). Known references to update (from Phase 0 discovery):

- `README.md:17,20,23-31,33,81,84,57` — usage instructions, the full sample `config.yaml` block with field comments, `vault.path` tilde/fail-fast note, project-layout listing (`config/ config.yaml loading`), and the `docs/config.yaml` dual-vault explanation.
- `docs/architecture.md:75` — one-line description: "`config` — loads and validates `config.yaml` (Vault path with `~` expansion, server port, default theme)."

## What to implement

1. `README.md`: replace the `config.yaml` usage section with: how to run the binary (`--port` flag, default 8080), and the new first-run experience (server boots with no Active Vault; open the browser, use the vault picker to select or add a vault; last selection persists in `~/.config/ks/settings.json` for next time). Remove the sample YAML block and the `docs/config.yaml` dual-vault explanation (that capability was dropped per ADR-0011's Consequences section — don't describe a feature that no longer exists).
2. `docs/architecture.md:75`: update the one-line `config` package description to reflect that `internal/config` no longer exists — describe `internal/settings` and `internal/activevault` in its place if the architecture doc's format calls for a per-package one-liner (match whatever structure is already there rather than inventing a new section format).
3. Double check `.gitignore:2`'s `/config.yaml` line was removed (should already be done in Ticket 04 — verify, don't re-do).

## Verification checklist

- `grep -rn "config.yaml" README.md docs/` returns nothing.
- `grep -rn "internal/config" docs/architecture.md` returns nothing (or is replaced with the new package names, per point 2).
- README's instructions, followed literally on a clean checkout, actually work (build, run with `--port`, open browser, see the vault picker) — do a real end-to-end pass, not just a text edit.

## Anti-pattern guards

- Don't leave stale "run with `-config path/to/config.yaml`" instructions anywhere — this is exactly the kind of doc drift that confuses the next reader.
- Don't describe the dropped concurrent-multi-instance/docs-as-second-vault capability as if it still works — it doesn't, per ADR-0011.

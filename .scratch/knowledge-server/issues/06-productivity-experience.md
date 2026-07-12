# 06 — Productivity Experience

Status: open
Blocked by: 05

## Goal

Ship the web renderer: the HTML/HTMX/Alpine.js frontend for browsing, searching, and visualizing the graph, plus a filesystem watcher for live updates.

## Deliverables

- `web/` — HTML/CSS + HTMX + Alpine.js UI for note browsing, search, and graph visualization (Cytoscape.js).
- `internal/watcher` — filesystem watcher that detects Vault changes and triggers incremental index/graph updates.
- Theme support (`theme.default` from config, currently parsed but unused — wired up here).

## Exit criteria

- Editing a note in the Vault is reflected in the UI without a server restart.
- Search and graph views are navigable in a browser.
- `theme.default` actually changes the rendered theme.

## Comments

Draft placeholder — original ticket content was lost; this is a fresh draft from the roadmap description in `spec.md`, not a reconstruction. Needs a grilling pass before implementation starts.

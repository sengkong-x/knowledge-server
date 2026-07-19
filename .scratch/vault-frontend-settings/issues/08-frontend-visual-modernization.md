---
title: "Modernize visual design: type scale, spacing, color system, motion — zero build step"
created: 2026-07-19
tags: [issue]
---

Status: resolved
Type: task

## Context

Hard constraint from the grill (Q10/A): **no build step**. Stay entirely within hand-written CSS/JS embedded via Go's `embed` (`web/assets.go`) plus the existing vendored `htmx.min.js`/`alpine.min.js`/`cytoscape.min.js` — no npm, no Tailwind, no bundler, no new JS dependencies. This ticket should run *after* Ticket 07 (nav/picker markup exists) so the redesign covers the real final markup, not a mockup of it.

Full current CSS state (all three files are small — read them, this is the entirety of what exists):
- `web/themes/base.css` (16 lines): styles only `body` (background/color/font-family/max-width/margin/padding), `a` (color), `#cy` (width/height/border). No `.button`, `nav`, `form`, or `input` styling exists anywhere.
- `web/themes/dark.css` / `web/themes/light.css` (6 lines each): each defines exactly 4 CSS custom properties under `:root[data-theme="..."]`: `--bg`, `--fg`, `--link`, `--border`.

Current markup has no nav (until Ticket 07 adds one), no buttons, no styled forms — `noteDetailTemplate`, `browseTemplate`, `searchUITemplate` are bare `<h1>`/`<ul>`/`<a>`/`<form>`/`<input>` with zero classes (server.go:76-117, cited in the spec's Phase 0 section).

## What to implement

This is a design-quality task, not a mechanical one — consider loading the `frontend-design` skill for aesthetic direction before writing CSS, since "modernize the look and feel" was the user's explicit, somewhat open-ended ask.

Concretely, within the existing 3-file CSS structure (extend `base.css` and the two theme files, don't restructure the embed/loading mechanism):

1. **Color system**: expand beyond the current 4 custom properties (`--bg`, `--fg`, `--link`, `--border`) to cover what real UI needs — a surface/elevated-surface distinction (for the new nav bar and any cards), a muted-text color, an accent/interactive-state color (hover/focus/active), and border-radius/shadow tokens if the direction calls for depth. Keep both `light.css` and `dark.css` internally consistent — same property names, different values, exactly like today's pattern.
2. **Typography**: a real type scale (not just `font-family` on `body`) — heading sizes for `h1`/`h2`, body line-height, and a monospace stack for anything code-like (note IDs, the graph view) if that fits the direction chosen.
3. **Spacing/layout rhythm**: consistent spacing scale (e.g. CSS custom properties `--space-1` through `--space-5`) instead of ad-hoc `rem` values, applied to the new nav (Ticket 07), buttons, forms, and existing content areas.
4. **New component styles** (didn't exist before): `nav`/header chrome for Ticket 07's picker, buttons (the "Add new vault..." toggle, submit, theme toggle), form/input styling (the vault path input, the existing bare search `<input>` from `searchUITemplate`).
5. **Motion**: subtle, purposeful transitions (e.g. theme-switch color transition, dropdown open/close, hover states) — CSS `transition`/`@keyframes` only, no JS animation library.
6. Add a `<meta name="viewport" content="width=device-width, initial-scale=1">` to `layoutTemplate`'s `<head>` (server.go:27-29) — currently absent, so the page isn't correctly responsive on mobile; small but real gap in the existing markup.

## Verification checklist

- Manually run the server and visually inspect Browse, Search, Graph, note detail, and the new vault-picker nav in **both** light and dark theme — check contrast, spacing consistency, and that the redesign reads as one coherent system rather than page-by-page patchwork.
- Confirm `#cy` (the Cytoscape graph container, base.css) still renders correctly — don't let the redesign accidentally break its existing width/height/border treatment or the `graph.js` styling it depends on (`web/js/graph.js`'s Cytoscape `style` array only sets node labels, so all visual styling of the graph *container* — not nodes — comes from CSS).
- No new files under `web/vendor/` and no `package.json`/`node_modules` introduced anywhere in the repo (`find . -name package.json` returns nothing) — confirms the zero-build-step constraint held.
- Existing golden-path pages (Browse, Search, note detail, Graph) all still render without console errors — check via `go run ./cmd` + a browser, not just a visual glance (open devtools console).

## Anti-pattern guards

- Don't add a CSS preprocessor (Sass/Less/PostCSS) — that implies a build step, which is explicitly out of scope.
- Don't add icon-font or web-font `@import`s from external CDNs — the project's whole design principle (per architecture memory) is a single small binary with no runtime dependency fetches; any icons/fonts must be inlined/embedded, not loaded from a CDN at runtime.
- Don't restructure `web/assets.go`'s embed directive or the file-serving routes (`/vendor/`, `/themes/`, `/js/`) — this ticket only changes CSS/markup content, not the serving mechanism.

---
title: Undirected edges for `related`-based graph
created: 2026-07-18
tags: [adr]
---

`internal/graph` builds a relationship graph from each Note's `related` frontmatter field. We considered treating `related` as a directed edge (A → B when A lists B, distinguishing "forward links" from "backlinks" the way many note-taking tools do) against treating it as declaring a single undirected edge between A and B. We chose undirected: a vault author writing `related: [B]` on note A means "A and B are connected," not "A points to B," and there's no forward/backlink distinction anywhere else in the ticket's query surface (neighbors, shortest path, orphan detection all read naturally as undirected graph operations). Undirected edges also make orphan detection well-defined with no extra rule: a note is an orphan iff it has zero edges after symmetrizing, regardless of which side declared the `related` entry.

## Consequences

If A declares `related: [B]` but B does not declare `related: [A]`, `Neighbors(B)` still returns A — the edge is shared, not owned by whichever note declared it first. `Upsert(id)` only needs to recompute `id`'s own outgoing `related` list; because edges are shared records rather than per-note copies, the other side of each edge updates as a side effect, with no cross-note rescan required. If a future ticket needs to distinguish "who declared the link" (e.g. a backlinks view that shows only edges *not* declared by the current note), that requires a real redesign of the edge representation (storing declaring-side per edge) — this decision assumes that need doesn't exist yet.

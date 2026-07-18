---
title: Linear substring scan for full-text search
created: 2026-07-15
tags: [adr]
---

`internal/search` needs to match any contiguous substring, including mid-word (e.g. "lock" matches "Clock"), against note titles/bodies/aliases within the spec's 50ms budget. We considered a trigram inverted index (precompute 3-character trigrams per note, intersect candidate sets at query time, then verify) against a simpler design: `SearchStore` persists each note's normalized (lowercased) text, and a query does a linear `strings.Contains` scan over every entry's cached text. We chose the linear scan: a personal vault is realistically hundreds to a few thousand notes, well within the 50ms budget for an in-memory scan, and it avoids the real implementation complexity of trigram extraction, set storage, and candidate verification for a benefit that doesn't materialize at this scale.

## Consequences

`SearchStore`'s persisted cache exists to avoid re-parsing notes from disk on every query, not to avoid the scan itself — the "index" in the name is about removing parse/I/O cost from the hot path. If vault size grows large enough that a linear scan misses the 50ms target, revisit with a trigram (or similar) index; the substring-matching *behavior* should stay the same, only the internal structure would change.

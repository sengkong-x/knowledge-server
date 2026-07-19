---
title: "Add a version header to the gob-encoded Engines cache"
created: 2026-07-19
tags: [issue]
---

Status: open
Type: task

## Context

Read `docs/adr/0004-gob-encoding-for-index-cache.md` and the 2026-07-19 update in `docs/adr/0010-cache-persistence-lifecycle.md` first. Per the grill, cache format mismatches must be treated as a cache miss (rebuild), never a decode panic or silently-wrong data — this is the existing philosophy (ADR-0004: "the index is disposable and rebuildable by design") and must be preserved, just now with an explicit version check instead of relying on gob's implicit tolerance.

Exact current Save/Load pairs (read these files before changing anything):
- `internal/index/index.go:68` (`Save`), `:79` (`Load`), `:122` (`LoadOrBuild`) — gob-encodes `idx.entries` (a `map[string]IndexEntry`) directly.
- `internal/search/search.go:140` (`Save`), `:151` (`Load`), `:83` (`LoadOrBuild`) — gob-encodes `ss.entries` (`map[string]searchEntry`) directly.
- `internal/graph/graph.go:177` (`Save`), `:188` (`Load`), `:41` (`LoadOrBuild`) — wraps in `graphData{Entries: map}` before gob-encoding.

All three `LoadOrBuild` bodies do the same thing: `if _, err := Load(...); err == nil { ...; return }`; `return Build(...)` on any error. That "any error → rebuild" behavior is what makes a version mismatch safe to plug in as just another `Load` error.

## What to implement

Do **not** add a version field to `IndexEntry`/`searchEntry`/`graphData` — gob tolerates added struct fields, so old files could still decode successfully with different semantics, which is the failure mode being avoided.

Instead, in each of the three `Save`/`Load` pairs, write/read a small fixed header *before* the gob stream:

```go
const cacheFormatVersion = 1 // bump on any change to the encoded shape

func writeCacheHeader(w io.Writer) error {
    _, err := w.Write([]byte(fmt.Sprintf("KSC%d\n", cacheFormatVersion)))
    return err
}

func readCacheHeader(r *bufio.Reader) error {
    line, err := r.ReadString('\n')
    if err != nil { return err }
    if line != fmt.Sprintf("KSC%d\n", cacheFormatVersion) {
        return errors.New("cache format version mismatch")
    }
    return nil
}
```

(Illustrative — pick whatever concrete implementation fits each `Save`/`Load` cleanly; the shape that matters is: a short marker written first, checked first on read, mismatch or read error returns a plain `error` from `Load` exactly as a missing/corrupt file would today, so no `LoadOrBuild` caller needs to change.) Apply this identically across all three packages — don't invent three different header formats.

Since `cacheFormatVersion` needs to live somewhere all three packages can share without a new inter-package dependency, either duplicate the tiny constant+helper in each package (matches this repo's existing preference for independent, disposable engines — see ADR-0011's "Engines" glossary entry in `CONTEXT.md`), or add one small shared package (e.g. `internal/cacheformat`) if duplication feels wrong. Prefer duplication unless the three copies would drift — this is a 2-line constant, not worth a new package.

## Verification checklist

- Existing tests in `internal/index/index_test.go`, `internal/search/search_test.go`, `internal/graph/graph_test.go` for `Save`/`Load` round-trips still pass unchanged (round-trip through the same version should be transparent).
- New test per package: write a cache file with a stale/garbage header, assert `Load` returns an error (not a panic, not successful-but-wrong data), and assert `LoadOrBuild` falls back to `Build()` cleanly in that case.
- `go test ./internal/index/... ./internal/search/... ./internal/graph/...` passes.

## Anti-pattern guards

- Don't add a version *field* to the payload structs — established above as insufficient.
- Don't make version mismatch a fatal error/panic — must degrade to "rebuild," per existing philosophy.
- Don't build a generic reusable "versioned gob codec" abstraction spanning three packages if a 2-line duplicated helper does the job — avoid premature abstraction.

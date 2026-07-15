# Gob encoding for on-disk index cache

The Index's on-disk form under `.cache/` needs a serialization format. We considered JSON (inspectable, easy to debug), gob (Go's native binary encoding, compact and fast), and SQLite (real queries, but a much heavier dependency for what's currently just ID/tag lookups). We chose gob: it's faster to encode/decode and more compact than JSON, and the index is disposable and rebuildable by design, so opacity and Go-only portability are acceptable trade-offs — nobody needs to hand-inspect or share this file across languages.

## Consequences

Because gob encodes concrete Go types directly, changes to `IndexEntry`'s shape can make old `.cache/` files fail to decode. This is fine as-is: the Index is designed to be deleted and rebuilt from the Vault at zero knowledge loss, so a decode failure should be handled by rebuilding, not migrating.

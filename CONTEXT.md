# Knowledge Server

A self-hosted personal knowledge system: a Go engine that reads, indexes, searches, and links notes stored as plain Markdown in a Vault. Single-context repo, organized around independent engines rather than features.

## Language

**Vault**:
The Markdown-and-assets directory on disk that is the sole source of truth for all knowledge. Every derived artifact (search index, graph cache, embeddings) is disposable and must be fully rebuildable from the Vault.
_Avoid_: repository, knowledge base (as a filesystem term)

**Note**:
A single Markdown file with YAML frontmatter representing one unit of knowledge in the Vault.
_Avoid_: document, page, article

**VaultProvider**:
The filesystem-level abstraction over the Vault. Discovers notes and reads raw bytes for notes and assets — no Markdown or frontmatter parsing. Backed by local disk today; designed so a future implementation could back onto Git, S3, or a remote vault without changing callers.
_Avoid_: FileStore, Repository

**NoteStore**:
The note-level abstraction that returns parsed `Note` values, built on top of `VaultProvider`. Lives in `internal/notes`; its implementation composes a `VaultProvider` with `internal/parser`'s `Parse` function.
_Avoid_: NoteRepository, DocumentStore

**NoteRef**:
A lightweight reference to a Note before it has been read or parsed — carries only its ID and relative path within the Vault. Returned by `VaultProvider.ListNotes()`.
_Avoid_: NoteStub, NoteHandle

**Index**:
A disposable, rebuildable projection of Vault metadata, built from `NoteStore`, that supports lookup by ID and by tag without re-parsing every Note. Lives in `internal/index`. Deleting it never loses knowledge — only rebuild cost.
_Avoid_: Database, cache (as the primary concept — the on-disk form is a cache, but "Index" is the concept)

**IndexEntry**:
A single Index record: a Note's ID, title, tags, path, and created timestamp — no Body. Distinct from `NoteRef`, which is pre-parse and carries only ID and path.
_Avoid_: NoteRef (that's the pre-parse form), NoteSummary

**SearchStore**:
A disposable, rebuildable structure holding normalized full text (title, body, and aliases) per Note, for substring search. Built from `NoteStore`, lives in `internal/search`. Distinct from `Index`: `Index` supports exact ID/tag lookup over metadata, `SearchStore` supports substring matching over text content. Matches any contiguous substring, including mid-word (e.g. "lock" matches "Clock") — not token/word-boundary search.
_Avoid_: Index (that term is reserved for `internal/index`), SearchIndex, Corpus

**Graph**:
A disposable, rebuildable undirected relationship graph over Notes, built from `NoteStore`, lives in `internal/graph`. Edges come only from the `related` frontmatter field, resolved by Note ID — not from aliases, and not from in-body links (a future engine's territory, see ADR-0006). `related: [B]` on note A declares a single shared edge between A and B regardless of whether B lists A back. Supports direct neighbor lookup, unweighted shortest path, and orphan detection (notes with zero edges).
_Avoid_: Backlinks (implies directionality this graph doesn't have), LinkGraph

**GraphEntry**:
A single Graph node: a Note's ID and its resolved set of neighbor IDs. `related` entries that don't resolve to an existing Note ID are dropped when building edges (not represented as phantom nodes) and reported in the graph's `BuildReport`, mirroring `index.BuildReport`. Self-references and duplicate `related` entries are silently normalized to no-ops.
_Avoid_: Node (too generic outside the graph package itself), GraphNode

**Watcher**:
Monitors the Vault for filesystem changes and drives incremental `Upsert`/`Remove` calls into `Index`, `SearchStore`, and `Graph` so they stay current without a full rebuild or a server restart. Lives in `internal/watcher`. Reacts only to Note files; asset changes don't affect any of the three engines and are ignored.
_Avoid_: Poller (the mechanism is event-driven, not polling), File watcher (redundant with Vault-scoping already implied)

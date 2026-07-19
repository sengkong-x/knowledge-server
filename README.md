# Knowledge Server

A self-hosted personal knowledge system: a Go engine that reads, indexes, searches, and links notes stored as plain Markdown in a Vault.

It's a personal "second brain" for continuous capture, organization, and reuse of knowledge from AI conversations, technical research, engineering experience, architecture decisions, interview prep, and learning notes — human-friendly to read, machine-friendly for AI agents, and simple enough to maintain for decades.

## Design principles

- **Vault as source of truth** — the Markdown files on disk are always authoritative. Everything derived (search index, graph cache, embeddings) is disposable and must be fully rebuildable from the Vault; deleting derived state must never lose knowledge.
- **Minimal deployment** — production is just a binary plus a vault directory, no database, Redis, message queue, or container requirement.
- **Engines, not features** — the codebase is organized around independent engines (vault, parser, search, graph, …), each with a single responsibility and a clean API. The HTTP layer orchestrates; it holds no business logic itself.

## Usage

```sh
go build -o ks ./cmd
./ks --port 8080
```

`--port` defaults to `8080` if omitted.

The server boots with no vault selected — there's no config file and no required startup argument for which Vault to open. Open the browser to the port above and use the vault picker in the nav bar to select an existing vault path or add a new one; switching vaults and toggling light/dark theme are both done from there at runtime. Your last selection (vault path, theme, and a short history of previously-opened vaults) is saved to `~/.config/ks/settings.json` and restored automatically the next time you start `./ks`.

### Health check

```sh
curl http://localhost:8080/health
```

```json
{"vault_path": "/home/you/knowledge", "note_count": 42}
```

## Development

```sh
go build ./...
go test ./...
```

## Project layout

```
cmd/            entry point — wires settings, ActiveVault, and server; no business logic
internal/
  settings/     durable settings.json persistence (active vault path, theme, vault history)
  activevault/  the single Vault a running instance currently has open, and switch orchestration
  vault/        VaultProvider — filesystem access to the Vault
  parser/       pure Markdown/frontmatter parsing into a Note
  notes/        NoteStore — VaultProvider + parser composed into parsed Note access
  index/        Index — ID/tag lookup over Vault metadata
  search/       SearchStore — full-text substring search over Vault content
  graph/        Graph — undirected relationship graph from notes' `related` field
  engines/      Index + SearchStore + Graph managed together as one unit
  state/        concurrency-safe wrapper around Engines, plus SSE subscribers
  watcher/      fsnotify-driven incremental updates into State
  server/       HTTP layer (stdlib net/http.ServeMux only, see docs/adr/0002)
  logger/       slog setup
web/            vendored frontend assets (HTMX, Alpine.js, Cytoscape.js, theme CSS)
docs/           architecture doc and ADRs
```

See `docs/architecture.md` for a full walkthrough (C4 diagrams, request lifecycle, persistence), `CONTEXT.md` for domain vocabulary, and `docs/adr/` for individual architectural decisions.

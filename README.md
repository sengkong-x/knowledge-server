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
./ks --config ./config.yaml
```

`--config` defaults to `./config.yaml` if omitted.

### Configuration

```yaml
vault:
  path: ~/knowledge   # ~ expands to the home directory; must exist and be a directory
server:
  port: 8080
theme:
  default: dark       # light or dark; drives which CSS file the renderer links
```

`vault.path` supports a leading `~` for the home directory. The server fails fast at startup if `vault.path` doesn't exist or isn't a directory.

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
cmd/            entry point — wires config, vault, engines, watcher, and server; no business logic
internal/
  config/       config.yaml loading
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
docs/           architecture doc, ADRs, and (see below) a second Vault for viewing them
```

See `docs/architecture.md` for a full walkthrough (C4 diagrams, request lifecycle, persistence), `CONTEXT.md` for domain vocabulary, and `docs/adr/` for individual architectural decisions.

## Viewing the documentation through `./ks`

The files under `docs/` (the architecture doc and every ADR) are themselves valid Notes — each carries the `title`/`created` frontmatter the parser requires — so `docs/` doubles as a second Vault. `docs/config.yaml` points at it:

```sh
go build -o ks ./cmd
./ks --config ./docs/config.yaml
```

This starts a second instance, browsable, searchable, and graph-viewable exactly like your real Vault, on its own port so it can run alongside your normal `./ks --config ./config.yaml` instance. `docs/config.yaml` is checked into the repo (unlike the root `config.yaml`, which is gitignored since it points at your personal Vault) — it ships with the project so this works right after cloning.

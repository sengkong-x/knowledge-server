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
  default: dark       # reserved for the renderer (Phase 5+); unused by the server today
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

Current (Phase 0 — Foundation & Architecture):

```
cmd/            entry point — wires config, vault, and server; no business logic
internal/
  config/       config.yaml loading
  vault/        VaultProvider — filesystem access to the Vault
  parser/       Note / NoteStore types (implementation lands in a later phase)
  server/       HTTP layer (stdlib net/http.ServeMux only, see docs/adr/0002)
  logger/       slog setup
web/            frontend assets (future)
```

The target layout adds one `internal/` package per engine as its phase lands — `markdown`, `index`, `metadata`, `search`, `graph`, `cache`, `watcher`, `api` — each introduced only when its ticket does, not scaffolded ahead of time.

## Roadmap

Each phase is one ticket under `.scratch/knowledge-server/issues/`, built sequentially:

| # | Phase | Ticket |
|---|-------|--------|
| 0 | Foundation & Architecture | ✅ done |
| 1 | Knowledge Reader | parser + `NoteStore` implementation |
| 2 | Knowledge Indexing | |
| 3 | Search Engine | |
| 4 | Knowledge Graph | |
| 5 | Productivity Experience | web renderer |
| 6 | AI Integration | AI context API |
| 7 | Interactive Knowledge | |

Backlog (unticketed): semantic search / embeddings, knowledge gap detection, generated learning roadmaps, plugin system. See `.scratch/knowledge-server/spec.md` for the full spec and rationale.

See `CONTEXT.md` for domain vocabulary and `docs/adr/` for architectural decisions.

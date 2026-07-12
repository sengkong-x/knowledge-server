# 01 — Foundation & Architecture

Status: done
Blocked by: (none)

## Goal

Create a clean, extensible foundation. No user-facing features yet — this establishes the project skeleton and core abstractions everything else builds on.

## Deliverables

Module: `github.com/sengkong/knowledge-server`, Go 1.26.

Project structure (no placeholder dirs for later-phase engines — those are created when their tickets land):

```
knowledge-server/
├── cmd/
├── internal/
│   ├── config/
│   ├── vault/
│   ├── parser/
│   ├── server/
│   └── logger/
└── web/
```

Tests are colocated `_test.go` files per package — no top-level `tests/` tree.

- **Configuration** — `knowledge-server.yaml` support, parsed with `gopkg.in/yaml.v3`:
  ```yaml
  vault:
    path: ~/knowledge
  server:
    port: 8080
  theme:
    default: dark
  ```
  Config path is passed via `--config` flag, defaulting to `./knowledge-server.yaml`. `vault.path` gets manual `~` expansion via `os.UserHomeDir()` (YAML unmarshaling doesn't do this automatically).
- **Logging** — stdlib `log/slog`, wrapped in a thin `internal/logger` package (constructor sets up the handler).
- **HTTP layer** — stdlib `net/http.ServeMux` only, no router dependency (see ADR-0002).
- **Filesystem abstraction** — a `VaultProvider` interface responsible for discovering notes, reading files, and loading assets, implemented now. Design it so a future implementation could back onto Git, S3, or a remote vault without changing callers:
  ```go
  type VaultProvider interface {
      ListNotes() ([]NoteRef, error)
      ReadNote(id string) ([]byte, error)
      ReadAsset(path string) ([]byte, error)
  }

  type NoteRef struct {
      ID   string // derived from relative path, e.g. "linux/process"
      Path string // relative path within the vault, e.g. "linux/process.md"
  }
  ```
- Core interfaces defined before any implementation — `NoteStore` is declared as an interface signature only; its implementation arrives in ticket 02 once the parser exists (see ADR-0001):
  ```go
  type NoteStore interface {
      List() ([]Note, error)
      Load(id string) (*Note, error)
  }
  ```
- Avoid putting logic into `main.go` — the HTTP layer should orchestrate engines under `internal/`, not contain business logic itself.

## Exit criteria

- Server starts successfully and fails fast at startup if `vault.path` doesn't exist or isn't a directory.
- Configuration loads from `knowledge-server.yaml`.
- A vault directory can be loaded via `VaultProvider`.
- `GET /health` returns `200 OK`, reporting the vault path and note count (via `VaultProvider.ListNotes()`).
- Clean package structure in place for subsequent phases to build into.

## Comments

Implemented — see commit "Implement foundation: config, vault provider, health endpoint, CI". `config.Load`, filesystem-backed `VaultProvider` (`ListNotes`/`ReadNote`/`ReadAsset`), `vault.ValidateRoot` for fail-fast startup, and `GET /health` are all in place and covered by tests in `internal/config`, `internal/vault`, and `internal/server`. `NoteStore` remains an interface-only declaration in `internal/parser`, per ADR-0001, pending ticket 02.

# Personal Knowledge Server — Spec

Source: converted from a ChatGPT design discussion (3 rounds of refinement), see `output.txt` at repo root for the raw transcript.

## 1. Vision

Build a lightweight, self-hosted, AI-friendly personal knowledge system for continuous capture, organization, exploration, and reuse of knowledge from AI conversations, technical research, engineering experience, architecture decisions, interview prep, learning notes, and project docs.

The system is a personal "second brain": human-friendly to read, machine-friendly for AI agents, portable, frontend-independent, and simple enough to maintain for decades.

## 2. Core design principles

**Separation of concerns** — three independent layers:

- **Knowledge Vault** (source of truth): Markdown, images, diagrams, metadata, attachments.
- **Knowledge Engine**: parser, indexer, search, graph builder, AI context API, cache builder.
- **Renderer**: web UI, search UI, graph visualization, themes, animations.

**Minimal deployment** — production is just a binary plus a vault directory:

```
knowledge-server
vault/
```

Run: `./knowledge-server --vault ./vault`. No database, Redis, message queue, Kubernetes, container requirement, or runtime install.

**Source of truth** — the vault (Markdown files) is always authoritative. Everything else (search index, graph cache, embeddings, summaries) lives under `.cache/` and must be fully disposable — deleting `.cache` must never lose knowledge.

## 3. Target architecture

**Backend**: Go — `net/http`, `goldmark` (Markdown), a YAML parser, a filesystem watcher, embedded assets. Static single-binary compilation, low memory, cross-platform.

**Frontend**: HTML/CSS + HTMX + Alpine.js. Optional: Cytoscape.js (graph), Mermaid (diagrams), Shiki (syntax highlighting). Avoid unnecessary frontend complexity.

**Codebase organized around engines, not features** — each engine has a single responsibility and a clean API; the HTTP layer orchestrates, it doesn't hold business logic:

```
knowledge-server/
├── cmd/
├── internal/
│   ├── config/
│   ├── vault/
│   ├── parser/
│   ├── markdown/
│   ├── index/
│   ├── metadata/
│   ├── search/
│   ├── graph/
│   ├── cache/
│   ├── watcher/
│   ├── api/
│   └── server/
├── web/
├── plugins/    (future)
├── themes/
└── templates/
```

## 4. Vault structure

```
vault/
├── linux/
│   ├── process.md
│   ├── memory.md
│   └── networking.md
├── database/
│   ├── postgres/
│   └── oracle/
├── distributed-systems/
├── architecture/
├── interview/
├── templates/
├── assets/
└── README.md
```

## 5. Knowledge format

Markdown with YAML frontmatter:

```markdown
---
title: Hybrid Logical Clock
tags:
  - distributed-system
  - consistency
aliases:
  - HLC
related:
  - vector-clock
  - lamport-clock
status:
  - evergreen
created: 2026-07-12
---

# Overview

Hybrid Logical Clock combines:
- physical time
- logical counter
...
```

## 6. Non-functional requirements

- **Performance**: startup < 1s, memory < 100MB, search < 50ms.
- **Reliability**: no database corruption; vault remains usable without the server; cache is always rebuildable; Git-friendly.
- **Security**: local-only initially; auth/HTTPS/multi-user are future work.

## 7. Success criteria

- Capturing new knowledge takes < 30 seconds.
- Finding existing knowledge takes < 10 seconds.
- AI agents can understand context, update notes, suggest relationships, and generate learning materials via the AI Context API.

## 8. Roadmap → tickets

The roadmap below is the "if this were my project" 6-phase v1, expanded to match the more detailed 8-phase breakdown from the final round of the discussion. Each phase is one ticket in `issues/`, built sequentially (each blocked by the previous):

| # | Ticket | Phase | Duration |
|---|--------|-------|----------|
| 01 | Foundation & Architecture | Phase 0 | ~1 week |
| 02 | Knowledge Reader | Phase 1 | 1-2 weeks |
| 03 | Knowledge Indexing | Phase 2 | ~1 week |
| 04 | Search Engine | Phase 3 | ~1 week |
| 05 | Knowledge Graph | Phase 4 | ~2 weeks |
| 06 | Productivity Experience | Phase 5 | ~2 weeks |
| 07 | AI Integration | Phase 6 | 2-3 weeks |
| 08 | Interactive Knowledge | Phase 7 | 3-4 weeks |

**Backlog (not ticketed yet — future work per the discussion):** Phase 8, "Advanced Knowledge Intelligence" — semantic search / embeddings, knowledge gap detection, generated learning roadmaps, and a plugin system (Timeline, Kanban, Mind Map, Flashcards, etc.). Revisit once Phase 7 ships; these need their own grilling pass since they weren't fully specified in the source discussion.

## Comments

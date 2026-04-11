# mem7

A lightweight MCP server in Go for shared memory across AI agents. Single binary, zero cgo, usable standalone over stdio or behind [agent-mesh](https://github.com/KTCrisis/agent-mesh) as a governed backend. `v0.2.0` introduces a hybrid markdown + SQLite store with full-text search and a dual stdio / HTTP transport.

## Features

- **6 MCP tools** — `memory_store`, `memory_recall`, `memory_search`, `memory_get`, `memory_list`, `memory_forget`
- **Hybrid storage** — append-only markdown workspace as source of truth, SQLite (FTS5) as a rebuildable index
- **BM25 full-text search** — zero embedding dependency, native FTS5 ranking, rich operators (`foo*`, `AND`, `OR`, `NOT`)
- **Dual transport** — same binary speaks MCP over stdio by default, or over HTTP JSON-RPC via `mem7 serve`
- **Snapshot reminder** — `POST /memory/snapshot_reminder` (and the matching MCP method) lets an agent runtime inject a pre-compaction instruction into its context
- **Automatic v0.1 migration** — legacy `memories.json` is imported into the new layout on first run and renamed `.v0.1.bak`
- **Rebuildable index** — `mem7 rescan` drops the SQLite index and replays the markdown workspace to restore consistency
- **Tag filters, agent tracking, TTL** — preserved from v0.1

## Quick start

```bash
go install github.com/KTCrisis/mem7/cmd/mem7@latest
```

Or build from source :

```bash
cd mem7
go build -o ~/go/bin/mem7 ./cmd/mem7
```

Default stdio mode (MCP client spawns the binary) :

```bash
~/go/bin/mem7
```

HTTP backend mode (shared across multiple clients) :

```bash
MEM7_TOKEN=mem7_secret123 ~/go/bin/mem7 serve --listen :9070
```

Rebuild the SQLite index from the markdown workspace :

```bash
~/go/bin/mem7 rescan
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MEM7_DIR` | `~/.mem7` | Data directory (hosts `workspace/` and `index.db`) |
| `MEM7_LISTEN` | `:9070` | HTTP bind address when in `serve` mode |
| `MEM7_TOKEN` | *(empty)* | Bearer token required on `/rpc` and `/memory/*` when set |
| `MEMORY_MAX_ENTRIES` | `10000` | Soft ceiling on live entries |
| `MEMORY_DIR` | — | Legacy alias for `MEM7_DIR`, still accepted |

Flags on `mem7 serve` mirror `MEM7_LISTEN` and `MEM7_TOKEN` : `--listen :9070 --token mem7_...`.

## Workspace layout

```
~/.mem7/
├── workspace/
│   ├── MEMORY.md                      # reserved for long-term notes
│   └── memory/
│       ├── 2026-04-11.md              # append-only daily logs
│       └── 2026-04-12.md
├── index.db                           # SQLite (facts + facts_fts)
└── memories.json.v0.1.bak             # one-shot backup if a v0.1 file was imported
```

The markdown files are the source of truth ; `index.db` is a derived cache that can be dropped and rebuilt from the markdown at any time via `mem7 rescan`.

Each entry is written as a level-2 heading followed by a fenced `mem7` envelope (plain key/value metadata) and a free-form body, terminated by a horizontal rule. A human can edit these files in place — the next `rescan` picks up the changes.

Example :

````markdown
## example_key

```mem7
op: store
agent: claude
tags: demo, example
created: 2026-04-11T20:00:00Z
updated: 2026-04-11T20:00:00Z
```

Free-form markdown content lives here.

---
````

## Usage with agent-mesh

In your `config.yaml` :

```yaml
mcp_servers:
  - name: memory
    transport: stdio
    command: /home/user/go/bin/mem7
    env:
      MEM7_DIR: /home/user/.mem7
```

agent-mesh discovers the tools via `tools/list` ; no per-tool wiring is required. Grants and policies apply as usual.

To share the same memory across several machines behind agent-mesh, run `mem7 serve` on one host and point the other hosts at it via the upcoming remote-client mode (Phase 1.5 of the roadmap).

## Tools

### memory_store

Upsert a memory entry by key. The markdown workspace receives an append-only section ; the SQLite index is updated in place.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | yes | Unique key for this memory |
| `value` | string | yes | Content to remember (free-form markdown allowed) |
| `tags` | string[] | no | Tags for filtering and grouping |
| `agent` | string | no | Identifier of the storing agent |
| `ttl` | number | no | Time-to-live in seconds (0 = permanent) |

### memory_recall

Recall memories by key, tags, or agent, most recently updated first.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | no | Exact key to recall |
| `tags` | string[] | no | Filter by tags (AND logic) |
| `agent` | string | no | Filter by agent |
| `limit` | number | no | Max results (default 10) |

### memory_search

Full-text search over memories using SQLite FTS5, ranked by BM25. Supports FTS5 operators : `foo*` prefix, `AND` / `OR` / `NOT`, quoted phrases.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | FTS5 query string |
| `tags` | string[] | no | Post-filter by tags |
| `agent` | string | no | Post-filter by agent |
| `since` | string | no | Lower bound on `updated_at` (RFC3339) |
| `until` | string | no | Upper bound on `updated_at` (RFC3339) |
| `limit` | number | no | Max results (default 10) |

### memory_get

Read a file from the markdown workspace, optionally between `from_line` and `to_line` (1-indexed, inclusive). Paths are resolved relative to the workspace root and refused if they escape it.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Workspace-relative path (e.g. `memory/2026-04-11.md`) |
| `from_line` | number | no | First line to read |
| `to_line` | number | no | Last line to read |

### memory_list

List memory keys with metadata (without values).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tags` | string[] | no | Filter by tags |
| `agent` | string | no | Filter by agent |

### memory_forget

Delete memories by key and/or tags. A tombstone section is appended to the markdown workspace, and the SQLite index soft-deletes the matching rows.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | no | Exact key to delete |
| `tags` | string[] | no | Delete all entries matching these tags (AND logic) |
| `agent` | string | no | Recorded on the tombstone |

## HTTP endpoints

`mem7 serve` exposes these routes :

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/healthz` | Liveness probe (always public, no auth) |
| `POST` | `/rpc` | JSON-RPC 2.0 endpoint — same MCP tool surface as stdio |
| `POST` | `/memory/snapshot_reminder` | Returns a structured instructional payload for an agent runtime to inject into its context before compaction |

Bearer auth is applied to `/rpc` and `/memory/*` when `MEM7_TOKEN` (or `--token`) is set.

Example :

```bash
curl -s -X POST http://localhost:9070/rpc \
  -H "Authorization: Bearer $MEM7_TOKEN" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call",
       "params":{"name":"memory_search","arguments":{"query":"roadmap*"}}}'
```

## Architecture

```
      Claude Code / agent-mesh / scripts
                    │
          MCP stdio ┴ MCP over HTTP
                    │
              ┌─────▼─────┐
              │ Dispatcher │   ← MCP protocol layer
              └─────┬─────┘
                    │
              ┌─────▼─────┐
              │   Store    │   ← orchestrator
              └──┬────┬───┘
                 │    │
          ┌──────▼┐ ┌─▼────────┐
          │markdown│ │ sqlite   │
          │workspace│ │ (facts + │
          │(truth) │ │ FTS5)    │
          └────────┘ └──────────┘
```

Every write goes through the markdown writer first and then updates the SQLite index. Reads consult the index only. If the index is corrupted or out of sync, `mem7 rescan` drops it and replays the markdown chronologically to reconstruct a consistent state.

## Migration from v0.1

On first startup, if `~/.mem7/memories.json` exists, `mem7` imports every entry into the new markdown + SQLite layout, renames the file to `memories.json.v0.1.bak`, and proceeds. The import is silent in stdio mode and logged to stderr in `serve` mode. No manual step is required.

## License

MIT

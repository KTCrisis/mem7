# memory-mcp-go

A lightweight MCP server in Go that provides shared memory between AI agents. Designed to work standalone or behind [agent-mesh](https://github.com/KTCrisis/agent-mesh) as a governed backend.

## Features

- **4 tools**: `memory_store`, `memory_recall`, `memory_list`, `memory_forget`
- **Shared state**: any agent can store and recall memories via MCP
- **Tag-based filtering**: organize memories with tags, filter with AND logic
- **Agent tracking**: each memory records which agent stored it
- **TTL support**: optional time-to-live with automatic expiration
- **Upsert**: storing with an existing key updates the entry
- **JSON persistence**: memories saved to a local JSON file
- **MCP stdio transport**: NDJSON over stdin/stdout, compatible with any MCP client

## Quick start

```bash
go build -o memory-mcp-go .
./memory-mcp-go
```

## Configuration

| Environment variable | Default | Description |
|---------------------|---------|-------------|
| `MEMORY_DIR` | `~/.memory-mcp` | Directory for the memories.json file |
| `MEMORY_MAX_ENTRIES` | `10000` | Maximum number of stored memories |

## Usage with agent-mesh

In your `config.yaml`:

```yaml
servers:
  memory:
    transport: stdio
    command: /path/to/memory-mcp-go
    env:
      MEMORY_DIR: /tmp/shared-memory
```

## Tools

### memory_store

Store or update a memory entry (upsert by key).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | yes | Unique key for this memory |
| `value` | string | yes | The content to remember |
| `tags` | string[] | no | Tags for filtering and grouping |
| `agent` | string | no | Identifier of the storing agent |
| `ttl` | number | no | Time-to-live in seconds (0 = permanent) |

### memory_recall

Recall memories by key, tags, or agent.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | no | Exact key to recall |
| `tags` | string[] | no | Filter by tags (AND logic) |
| `agent` | string | no | Filter by agent identifier |
| `limit` | number | no | Max results (default 10) |

### memory_list

List memory keys with metadata (without values).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tags` | string[] | no | Filter by tags |
| `agent` | string | no | Filter by agent |

### memory_forget

Delete memories by key or tags.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | no | Exact key to delete |
| `tags` | string[] | no | Delete all matching entries |

## Multi-agent example

```
Agent A (Claude)  ──store──►  memory-mcp-go  ◄──recall──  Agent B (Ollama)
                                    │
                              memories.json
```

Multiple agents sharing context through a common memory store, governed by agent-mesh policies.

## License

MIT

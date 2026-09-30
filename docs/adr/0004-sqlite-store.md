# ADR 0004: SQLite store layout and write-behind

Status: accepted, 2026-09-29.

## Decision

- One table per domain type (`workspaces`, `tasks`, `worktrees`, `sessions`), each a key column plus a `data` column holding the JSON-encoded domain struct. Workspaces are keyed by `root`, the rest by `id`.
- Migrations are `internal/adapters/sqlite/migrations/NNNN_name.sql`, embedded, forward-only, applied in one transaction on `Open`. `schema_version` holds a single row with the current version. A new migration gets the next number; existing ones are never edited.
- Upgrade tests open a committed fixture DB. `testdata/v1.db` is built from `testdata/v1.sql`; add a fixture for a version when a later migration changes its tables.
- `Put*` marshals and enqueues, coalescing by key. One writer goroutine commits pending rows in a single transaction 50 ms after the first unflushed put. `Flush` forces a commit and returns any write error since the last flush.
- WAL journal with `synchronous=NORMAL`: a killed process keeps every committed transaction.

## Why

- JSON rows mean adding a field to a domain type needs no migration. The daemon never queries by field; it loads everything on start.
- A 50 ms timer keeps loss on kill under the 100 ms budget while batching bursts of hook events into one transaction.

## Rejected

- **Column per field:** a migration per domain field change, for queries nothing runs.
- **`synchronous=FULL`:** only helps on power loss, and costs an fsync per commit.

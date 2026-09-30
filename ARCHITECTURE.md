# Architecture

This covers how `agentws` is built. What it does is in [FEATURES.md](FEATURES.md).

## Stack

- **Language:** Go (current stable). It builds one static binary that is the daemon, the TUI, the CLI and the hook handler. The reasons are in [docs/adr/0001-go.md](docs/adr/0001-go.md).
- **TUI:** Bubble Tea v2, Lip Gloss and Bubbles (charmbracelet).
- **Syntax highlighting:** chroma. Diffs are parsed from `git diff` output, not computed in Go.
- **Git:** the `git` CLI via exec, never a Go git library. It is the only thing that handles worktrees, sparse checkouts and user config correctly.
- **GitHub:** `cli/go-gh`, which reuses the user's `gh` auth. GraphQL with ETag/backoff for polling.
- **Terminals:** the `tmux` CLI against a dedicated server (`tmux -L agentws`).
- **Storage:** SQLite via `modernc.org/sqlite` (no cgo), with embedded migrations. Stored under `~/.agentws/`.
- **IPC:** a Unix socket at `~/.agentws/agentws.sock` carrying newline-delimited JSON. Request/response calls, plus a subscribe stream for state updates.
- **Notifications:** `osascript` in v1. A native helper can replace it later behind the same port.
- **nvim:** a small Lua plugin in `nvim/` that talks to the daemon through `agentws` CLI calls.
- **Tooling:** `go test`, `golangci-lint`, `testscript` for CLI end-to-end tests, GitHub Actions on macOS, goreleaser later.

## Layers

```
cmd/agentws            main: subcommands (daemon, tui, hook, new, cleanup, ...)
internal/domain        pure types and rules. No IO, no imports from other internal packages.
internal/app           use cases + ports (interfaces the use cases need)
internal/adapters/...  tmux, git, github, claude, codex, sqlite, notify, procs (lsof/ports), fs
internal/daemon        socket server, event loop, workers; wires adapters into app
internal/rpc           protocol types + client (used by tui, hook, cli, nvim)
internal/tui           Bubble Tea models; talks only to rpc.Client
nvim/                  Lua plugin
```

Dependency rule: `domain` ← `app` ← `adapters`/`daemon`, and `tui` → `rpc` only. `golangci-lint depguard` enforces it, so breaking it fails CI.

**Domain** (`internal/domain`): `Workspace`, `Repo`, `Task`, `Session`, `Worktree`, `Harness`, `AgentState`, `Usage`, `ReviewDraft`, `Comment`, `CleanupPlan`. Rules live here as plain functions:
- `Session.Apply(event)`: the state machine.
- `NameFor(task, prs)`: naming precedence.
- `PlanCleanup(worktrees, facts)`: the cleanup decision.
- Discovery rules.

Everything here is table-tested, with no mocks.

**App** (`internal/app`): use cases such as `StartSession`, `IngestHookEvent`, `AttachWorktree`, `RefreshPRs`, `PlanCleanup`/`ExecuteCleanup`, `BuildReview`, `SendReview`. They depend on port interfaces (`TerminalHost`, `Git`, `GitHub`, `HarnessAdapter`, `Store`, `Notifier`, `ProcessTable`) that the adapters implement. Tests use in-memory fakes.

## Staying fast

Clean layers must not cost latency, so these rules apply:

- **One state owner.** A single goroutine event loop in the daemon owns in-memory state. SQLite is written in the background after each change. Reads never hit disk.
- **Push, don't poll, for agent state.** Hooks call `agentws hook`, which writes one message to the socket and exits. It never waits on the daemon for more than 50 ms and never blocks the agent. If the daemon is down, the event is dropped and a log line is written.
- **TUI renders from a snapshot.** The daemon pushes state diffs, and the TUI never runs git, gh or tmux on the render path.
- **Heavy work goes to workers:** `du`, cleanup, diffs, and PR polling run in a bounded pool with debounce. Diffs are cached by tree hash.
- **Batch git:** one `git status --porcelain=v2 -z` per worktree per change burst, triggered by fsnotify with a 300 ms debounce.
- **Poll GitHub politely:** ETags, 60 s base interval, faster only for worktrees whose checks are running.

**Budgets** (CI benchmarks enforce these where possible):

| Path | Budget |
|---|---|
| `agentws hook` process wall time | < 20 ms p95 |
| Hook event → sidebar updated | < 150 ms |
| Keypress → frame | < 16 ms |
| Switch session (swap pane) | < 60 ms |
| Open review for a 50-file diff | < 300 ms |
| Daemon idle CPU | < 0.5% |

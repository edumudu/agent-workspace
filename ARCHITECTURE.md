# Architecture

This covers how `agentws` is built. What it does is in [FEATURES.md](FEATURES.md).

## Stack

- **Language:** Go (current stable). It builds one static binary that is the daemon, the TUI, the CLI and the hook handler. The reasons are in [docs/adr/0001-go.md](docs/adr/0001-go.md).
- **TUI:** Bubble Tea v2, Lip Gloss and Bubbles (charmbracelet).
- **Syntax highlighting:** chroma. Diffs are parsed from `git diff` output, not computed in Go.
- **Git:** the `git` CLI via exec, never a Go git library. It is the only thing that handles worktrees, sparse checkouts and user config correctly.
- **GitHub:** `cli/go-gh`, which reuses the user's `gh` auth. GraphQL with ETag/backoff for polling.
- **Terminals:** the `tmux` CLI against a dedicated server (`tmux -L agentws`) with its own config, driven only by `internal/adapters/tmux`. Panes are parked in their own windows and `swap-pane` puts one in the client's main slot. See [docs/adr/0003-tmux-terminal-host.md](docs/adr/0003-tmux-terminal-host.md).
- **Storage:** SQLite via `modernc.org/sqlite` (no cgo), with embedded migrations and write-behind. Stored at `~/.agentws/state.db` (`$AGENTWS_HOME/state.db` if set). See [docs/adr/0004-sqlite-store.md](docs/adr/0004-sqlite-store.md).
- **IPC:** a Unix socket at `~/.agentws/agentws.sock` carrying newline-delimited JSON. Request/response calls, plus a subscribe stream for state updates. See [Daemon and RPC](#daemon-and-rpc) and [docs/adr/0005-daemon-rpc.md](docs/adr/0005-daemon-rpc.md).
- **Notifications:** `osascript` in v1. A native helper can replace it later behind the same port.
- **nvim:** a small Lua plugin in `nvim/` that talks to the daemon through `agentws` CLI calls.
- **Tooling:** `go test`, `golangci-lint`, `testscript` for the e2e suite, `gremlins` for mutation testing, a `tdd` CI job that runs new tests against the base branch, `scripts/lint-comments`, GitHub Actions on macOS, and goreleaser later. Rules: [AGENTS.md](AGENTS.md).

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
scripts/               repo tooling: lint-comments (Go AST check), tdd-check, mutate
test/e2e               testscript suite; builds the binary and runs testdata/script/*.txtar
```

Dependency rule: `domain` ← `app` ← `adapters`/`daemon`, and `tui` → `rpc` only. `golangci-lint depguard` enforces it (rules in `.golangci.yml`), so breaking it fails CI. `tui` may import `domain` types.

`cmd/agentws` dispatches subcommands with the standard library; see [docs/adr/0002-cli-and-ci-tooling.md](docs/adr/0002-cli-and-ci-tooling.md). The version and commit come from `-ldflags -X main.version/main.commit`, falling back to the Go build info's VCS revision.

**Domain** (`internal/domain`): `Workspace`, `Repo`, `Task`, `Session`, `Worktree`, `Harness`, `AgentState`, `Usage`, `ReviewDraft`, `Comment`, `CleanupPlan`. Rules live here as plain functions:
- `Session.Apply(event) (Session, []Effect)`: the state machine. Adapters map hooks to harness-neutral events (`session_start`, `user_prompt_submit`, `pre_tool_use`, `post_tool_use`, `permission_request`, `waiting_for_input`, `stop`, `session_end`). Tool, permission and waiting events that arrive while `idle` or `done` are stale and ignored. `done` marks the session unread only when it is not focused; `Focus()` clears it.
- `NameFor(task, prs)`: naming precedence.
- `PlanCleanup(worktrees, facts)`: the cleanup decision.
- Discovery rules.

Everything here is table-tested, with no mocks.

**App** (`internal/app`): use cases such as `StartSession`, `IngestHookEvent`, `AttachWorktree`, `RefreshPRs`, `PlanCleanup`/`ExecuteCleanup`, `BuildReview`, `SendReview`. They depend on port interfaces (`TerminalHost`, `Git`, `GitHub`, `HarnessAdapter`, `Store`, `Notifier`, `ProcessTable`) that the adapters implement. Tests use in-memory fakes.

## Daemon and RPC

**Process.** `agentws daemon` runs in the foreground until SIGINT/SIGTERM. `agentws daemon start` spawns it detached (`setsid`, output to `$AGENTWS_HOME/daemon.log`) and waits for the socket. `daemon status` prints pid, uptime, and session and worktree counts; `daemon stop` sends SIGTERM and waits for the pid file to go. Files in `$AGENTWS_HOME` (default `~/.agentws`):

| File | Purpose |
|---|---|
| `agentws.lock` | `flock` held for the daemon's life; the kernel drops it if the daemon dies. A second daemon exits 1 with `already running (pid N)`. |
| `agentws.pid` | written after taking the lock, removed on clean exit |
| `agentws.sock` | the socket, mode 0600; a stale one is removed by the next lock holder |
| `state.db` | the SQLite store; loaded once on start |

**Event loop** (`internal/daemon`). One goroutine owns all state. Adapters call `Daemon.Post(event)` with `WorkspaceChanged`, `TaskChanged`, `WorktreeChanged` or `SessionChanged`; each one replaces the entity by key, enqueues a store write, bumps `seq` and fans the diff out to every subscriber in order. Connections read state only through closures run on the loop. Each connection has a 1024-message outbox; one that falls behind is disconnected, never waited on.

**Protocol** (`internal/rpc`, types in `protocol.go`). One JSON object per line, at most 16 MiB. Every message carries `"v":1`.

```
→ {"v":1,"id":1,"method":"status"}
← {"v":1,"id":1,"result":{"pid":42,"started_at":"…","sessions":2,"worktrees":3}}
→ {"v":1,"id":2,"method":"subscribe"}
← {"v":1,"id":2,"result":{"seq":17,"workspaces":[…],"tasks":[…],"worktrees":[…],"sessions":[…]}}
← {"v":1,"id":2,"diff":{"seq":18,"session":{…}}}
→ {"v":2,"id":3,"method":"status"}
← {"v":1,"id":3,"error":{"code":"unsupported_version","message":"this daemon speaks protocol v1"}}
```

- The client picks `id`; responses and a subscription's diffs carry it back. A connection may have several requests in flight.
- `subscribe` answers with the full `State` at `seq`, then one `diff` per change starting at `seq+1`. A diff sets exactly one of `workspace`, `task`, `worktree`, `session`, and replaces that entity by key. Domain structs encode with their Go field names.
- Error codes: `unsupported_version` (missing or other `v`), `unknown_method`, `bad_request` (not JSON; `id` 0).
- `hook` carries one harness hook: `{"harness","event","pane","at","payload"}`, with the hook's stdin JSON as `payload`. The daemon maps `pane` to the session whose `Pane` matches (`domain.SessionOnPane`) and the name to a harness event (`domain.HookEvent`), then applies it; unknown panes and names are ignored. The result is a `HookReply` whose optional `output` the hook prints for the harness.
- Adding a method or an optional field keeps `v:1`. Removing or changing the meaning of a field bumps `v`.

**Client.** `rpc.Dial(path)` connects; `rpc.Connect(ctx, path, start)` calls `start` once if nothing listens and retries for `rpc.StartTimeout` (2 s). `Client.Call(ctx, method, params, out)` is the generic call and returns `*rpc.Error` for daemon errors; `Status` and `Subscribe` wrap it. `Subscribe` returns the `State` and a `Diffs` channel closed when the connection ends; its diffs share the connection's reader, so a slow consumer should use its own `Client`. `rpc` cannot exec, so the caller supplies `start` (`cmd/agentws` spawns `agentws daemon`).

## Staying fast

Clean layers must not cost latency, so these rules apply:

- **One state owner.** A single goroutine event loop in the daemon owns in-memory state. SQLite is written in the background after each change. Reads never hit disk.
- **Push, don't poll, for agent state.** Hooks call `agentws hook`, which writes one message to the socket and exits. It never waits on the daemon for more than 50 ms and never blocks the agent. If the daemon is down, the event is dropped and a log line is written to `hook.log`. The hook dials the socket directly instead of using `rpc.Client`, sends one `hook` request and closes without reading the reply, except for events whose stdout the harness reads (`UserPromptSubmit`), where it waits up to the same 50 ms and prints nothing on timeout. `scripts/bench-hook.sh` checks the wall-time budget in CI. See [docs/adr/0006-hook-ingestion.md](docs/adr/0006-hook-ingestion.md).
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

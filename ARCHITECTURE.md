# Architecture

This covers how `agentws` is built. What it does is in [FEATURES.md](FEATURES.md).

## Stack

- **Language:** Go (current stable). It builds one static binary that is the daemon, the TUI, the CLI and the hook handler. The reasons are in [docs/adr/0001-go.md](docs/adr/0001-go.md).
- **TUI:** Bubble Tea v2, Lip Gloss and Bubbles (charmbracelet).
- **Syntax highlighting:** chroma. Diffs are parsed from `git diff` output, not computed in Go.
- **Git:** the `git` CLI via exec, never a Go git library. It is the only thing that handles worktrees, sparse checkouts and user config correctly.
- **GitHub:** `cli/go-gh`, which reuses the user's `gh` auth. GraphQL with ETag/backoff for polling.
- **Terminals:** the `tmux` CLI against a dedicated server (`tmux -L agentws`) with its own config, driven only by `internal/adapters/tmux`. Panes are parked in their own windows and `swap-pane` puts one in the client's main slot. See [docs/adr/0003-tmux-terminal-host.md](docs/adr/0003-tmux-terminal-host.md).
- **Config:** `github.com/BurntSushi/toml` reads the per-repo `.agentws.toml` setup recipe.
- **Storage:** SQLite via `modernc.org/sqlite` (no cgo), with embedded migrations and write-behind. Stored at `~/.agentws/state.db` (`$AGENTWS_HOME/state.db` if set). See [docs/adr/0004-sqlite-store.md](docs/adr/0004-sqlite-store.md).
- **IPC:** a Unix socket at `~/.agentws/agentws.sock` carrying newline-delimited JSON. Request/response calls, plus a subscribe stream for state updates. See [Daemon and RPC](#daemon-and-rpc) and [docs/adr/0005-daemon-rpc.md](docs/adr/0005-daemon-rpc.md).
- **Notifications:** `osascript` in v1, behind the `app.Notifier` and `app.Foreground` ports. A native helper can replace it later. See [Attention](#attention) and [docs/adr/0016-notifications-and-attention.md](docs/adr/0016-notifications-and-attention.md).
- **nvim:** a small Lua plugin in `nvim/` that talks to the daemon through `agentws` CLI calls.
- **Tooling:** `go test`, `golangci-lint`, `testscript` for the e2e suite, `gremlins` for mutation testing, a `tdd` CI job that runs new tests against the base branch, `scripts/lint-comments`, GitHub Actions on macOS, and goreleaser later. Rules: [AGENTS.md](AGENTS.md).

## Layers

```
cmd/agentws            main: subcommands (daemon, tui, hook, setup, new, cleanup, ...)
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
- `Session.Apply(event) (Session, []Effect)`: the state machine. Adapters map hooks to harness-neutral events (`session_start`, `user_prompt_submit`, `pre_tool_use`, `post_tool_use`, `permission_request`, `waiting_for_input`, `stop`, `session_end`, `subagent_start`, `subagent_stop`). Subagent events count as progress inside the turn, like tool events. Tool, permission and waiting events that arrive while `idle` or `done` are stale and ignored. `done` marks the session unread only when it is not focused; `Focus()` clears it.
- `NameFor(task, prs)`: naming precedence.
- `BannerFor(session, name, effect)` and `Coalescer`: which notify effects become a banner (not for muted sessions) and the one-per-10-s rule per session.
- `PlanCleanup(worktrees, facts)`: the cleanup decision.
- `Quotas(sessions)`, `Quota.Low`/`Stale`, `Advise(quotas, harness)`: the usage bar and the low-quota warning. See [docs/adr/0017-usage-and-limits-bar.md](docs/adr/0017-usage-and-limits-bar.md).
- Discovery rules: `KindOfRoot`, `ReposIn`, `SingleRepo`, `MergeRepoState`, `LastUsedWorkspace`. See [Workspaces](#workspaces).
- Worktree rules: `IsWorktreeAdd`, `SubagentParent`, `AttributeWorktree`, `ReconcileWorktrees`, `RollupChecks`, `PRForBranch`. See [Worktrees](#worktrees).
- Ports rules: `PortsByWorktree`, `KillGroups`. See [Ports](#ports).

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

**Event loop** (`internal/daemon`). One goroutine owns all state. Adapters call `Daemon.Post(event)` with `WorkspaceChanged`, `TaskChanged`, `WorktreeChanged` or `SessionChanged`; each one replaces the entity by key (`WorkspaceRemoved` deletes it), enqueues a store write, bumps `seq` and fans the diff out to every subscriber in order. Connections read state only through closures run on the loop. Each connection has a 1024-message outbox; one that falls behind is disconnected, never waited on.

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
- `subscribe` answers with the full `State` at `seq`, then one `diff` per change starting at `seq+1`. A diff sets exactly one of `workspace`, `task`, `worktree`, `session`, and replaces that entity by key. The one exception is a hook, whose diff sets `session` and `event` together; `event` is appended to its session's events. `State.events` holds each session's last 20. Domain structs encode with their Go field names.
- `hook` carries one harness hook: `{"harness","event","pane","at","payload"}`, with the hook's stdin JSON as `payload`. The daemon maps `pane` to the session whose `Pane` matches (`domain.SessionOnPane`) and the name to a harness event (`domain.HookEvent`), then applies it; unknown panes and names are ignored. The result is a `HookReply` whose optional `output` the hook prints for the harness.
- A diff may instead set `removed_workspace` (a root) or `removed_worktree` (an ID): drop that entity.
- A subagent change is a diff of its own that sets `subagent`, replacing the subagent with the same `SessionID` and `ID`. `State.subagents` holds every session's subagents (at most 30 each, in memory only). See [docs/adr/0019-subagent-tree.md](docs/adr/0019-subagent-tree.md).
- `statusline` carries one status-line update: `{"pane","report"}`. The daemon applies it with `Session.Report` to the session on that pane. `session.launch` (`{"harness","dir","model","effort","name","prompt"}` → `Session`) opens a pane through the harness adapter and adds an `idle` session on it; a non-empty `name` also creates a text task with that name, so the session and its banners carry it. It exists only with `WithHarnesses`, and fails with `launch_failed` if the pane cannot be created.
- `session.new` (`{"workspace","work_item","harness","model","effort"}` → `Session`) starts a session: it parses the work item, plans the dir and worktree in `domain`, then runs git, the setup recipe and tmux on the connection goroutine and commits the task, worktree, session and last-used workspace. `session.end` (`{"id"}` → `Session`) kills the pane and idles the session. With a terminal host and an open client layout, `session.focus` also swaps the session's pane into the main slot and focuses it. They need `WithHarnesses` and, for `session.new`, `WithSessions`. See [Sessions](#sessions).
- Methods: `status`, `subscribe`, `hook`, `statusline`, `session.launch`, `session.mute` (`{"id","muted"}`), `session.focus` (`{"id"}`), `session.new`, `session.end`, and `workspace.add` (`{"path": abs}` → `Workspace`), `workspace.list` (→ `{"workspaces": [...], "last_used": root}`), `workspace.remove` (`{"root": …}`), and `worktree.assign` (`{"id", "session"}`; an empty session unassigns). The workspace methods exist only when the daemon is built with `WithWorkspaces`; otherwise they answer `unknown_method`.
- Error codes: `unsupported_version` (missing or other `v`), `unknown_method`, `bad_request` (not JSON, bad params, or a path that is not a directory; `id` 0 when not JSON), `not_found` (removing an unknown workspace, or muting, focusing or ending an unknown session), `unavailable` and `failed` (below), `launch_failed`.
- `client.open` `{"command":[…],"env":{…}}` returns `{"slot","attach"}`: the client window (TUI pane on the left running `command`, main slot on the right), created on the first call and reused while it exists, plus the argv that attaches a terminal to it. `client.focus_main` makes that window's main slot the active pane. Both run tmux on the connection goroutine, never on the loop. Without a terminal host they return `unavailable`; a tmux failure returns `failed`.
- `debug.seed` `{"count":N}` adds N fake sessions (two per task, one to three worktrees each) for manual testing; `agentws debug seed N` calls it.
- Adding a method or an optional field keeps `v:1`. Removing or changing the meaning of a field bumps `v`.

**Client.** `rpc.Dial(path)` connects; `rpc.Connect(ctx, path, start)` calls `start` once if nothing listens and retries for `rpc.StartTimeout` (2 s). `Client.Call(ctx, method, params, out)` is the generic call and returns `*rpc.Error` for daemon errors; `Status` and `Subscribe` wrap it. `Subscribe` returns the `State` and a `Diffs` channel closed when the connection ends; its diffs share the connection's reader, so a slow consumer should use its own `Client`. `rpc` cannot exec, so the caller supplies `start` (`cmd/agentws` spawns `agentws daemon`).

## Workspaces

`agentws workspace add <path>`, `list` and `remove <path>` call `workspace.add`, `workspace.list` and `workspace.remove` on the daemon. See [docs/adr/0007-workspace-discovery.md](docs/adr/0007-workspace-discovery.md).

- **Kind.** `<path>/.git` a directory means `single`, with the path itself as the only repo. Anything else is an `orchestration` root: its direct children are scanned, symlinks followed, and each child with a `.git` directory is a repo. A child whose `.git` is a file is a worktree and is skipped, as is a dangling link. Nothing deeper than one level is read. Repos are sorted by name, and a symlinked repo keeps the link's name and path.
- **Layers.** The rules are pure functions in `domain`. `app.DiscoverWorkspace` uses the `WorkspaceFS` port (`adapters/fs`, stat and readdir only, no git). `app.RefreshRepoFacts` uses the `RepoInspector` port (`adapters/git`, at most 4 repos in flight).
- **Repo facts.** Default branch from `origin/HEAD` (empty without one), current branch (empty when detached) and changed-file count, from one `git status --porcelain=v2 --branch -z` plus one `git symbolic-ref` per repo.
- **Background refresh.** `workspace.add` answers from the filesystem alone, publishes the workspace, then refreshes facts off the loop and publishes again only if something changed. The daemon repeats that for every workspace every 30 s. Discovery and git run on connection goroutines and refresh workers, never the event loop.
- **Last used.** `Workspace.LastUsed` is set on every `add`, stored with the workspace, and `workspace.list` returns the most recent as `last_used`. The new-session dialog defaults to it.
- **Budget.** Discovery over 15 repos must finish in < 300 ms; `BenchmarkDiscovery` fails above that.

## TUI

`agentws` with no arguments asks the daemon for the client layout (`client.open`) and `exec`s the tmux attach argv it returns. The layout's left pane runs `agentws tui`, 48 columns wide; a `window-resized` hook on the window puts it back to 48 when the terminal resizes. See [docs/adr/0008-tui-shell.md](docs/adr/0008-tui-shell.md).

`agentws tui` (`internal/tui`) opens two connections: one subscribes and feeds diffs to the Bubble Tea program, the other makes calls such as `client.focus_main`, so a burst of diffs never delays a keypress. The model keeps the snapshot in maps and rebuilds the sidebar rows only when a diff arrives; grouping and order come from `domain.Sidebar` (sessions that need you first in each task group). Keys only move the selection, and `View` renders from memory. The selected session's card (task, PR chips, last 3 tool calls, what it waits on) is built by `domain.BuildSessionCard` from the events the model holds; see [docs/adr/0014-session-card.md](docs/adr/0014-session-card.md). Under the top bar, one row per harness shows its quota windows (percent left, time to reset), derived from the sessions' `Limits` by `domain.Quotas`; red below 20% left, dimmed with an age when older than 15 minutes, absent without data. See [docs/adr/0017-usage-and-limits-bar.md](docs/adr/0017-usage-and-limits-bar.md). Each session row lists its subagents underneath as a tree (`domain.SubagentTree`, at most 8 rows), collapsed with the worktrees by `o`. One 200 ms ticker drives every running spinner and the clock. The renderer runs at 120 fps: at the default 60 a key can wait a whole 16 ms frame before it is drawn.

Colors are Catppuccin Latte, overridden per key in the `[theme]` table of `$AGENTWS_HOME/config.toml` (`text`, `subtext`, `overlay`, `surface`, `mantle`, `base`, `blue`, `peach`, `green`, `red`, `teal`, `mauve`, `selected`), read once at startup.

## Attention

The daemon performs the effects `Session.Apply` returns. `EffectNotify` becomes a `domain.Banner` on the loop (`BannerFor`, then `Coalescer`), with no IO. A worker reads a bounded queue and calls `app.Notifier`; for a focused session it first asks `app.Foreground` whether a terminal app is in front, and drops the banner if so. `adapters/notify` implements both with `osascript`. Muted sessions get no banner and still go unread.

- Title is the session name (`NameFor`, else the harness), body is `needs permission`, `waiting` or `done`. At most one banner per session per 10 s.
- `$AGENTWS_HOME/notify.json` sets an optional macOS sound per event: `{"sounds":{"permission":"Glass"}}`.
- `session.mute` sets `Session.Muted` (the TUI's `m`). `session.focus` sets `Session.Focused` and clears unread (the TUI's `enter`); focus is cleared on daemon start.
- Claude and Codex share one path, and a fixture-driven daemon test covers both. See [docs/adr/0016-notifications-and-attention.md](docs/adr/0016-notifications-and-attention.md).

## Sessions

See [docs/adr/0015-session-lifecycle.md](docs/adr/0015-session-lifecycle.md).

- **Start.** `n` in the TUI or `agentws new [--workspace p] [--harness h] [--model m] [--effort e] <work item>` calls `session.new`. The work item is a Linear issue URL, a GitHub PR URL, or text; a URL of neither shape stays text (`domain.ParseWorkItem`). A single repo gets one worktree at `$AGENTWS_HOME/worktrees/<repo>/<slug>` branched from `origin/<default>` and its setup recipe run; an orchestration root starts at the root with no worktree (`domain.PlanSessionStart`). The work item is the agent's first prompt.
- **Low quota.** Under the harness row the dialog shows `domain.Advise`'s warning; `ctrl+s` switches to the other harness when it has reported limits.
- **Focus.** `enter`, and every new session, calls `session.focus`, which swaps the pane into the main slot.
- **End.** `x` then `y` calls `session.end`: the pane is killed and the session goes `idle` with no pane. It and its worktrees stay listed until cleanup.
- **Survival.** The daemon and the tmux server own sessions, so quitting the TUI or detaching changes nothing. On daemon start, restored sessions whose pane is gone are ended (`app.ReconcilePanes`), off the loop.
- **Budget.** `session.new` plus `session.focus`, excluding the setup recipe, must finish in < 1 s; the `NewSession` integration test checks it.

## Harness adapters

`app.HarnessAdapter` turns an `app.LaunchRequest` into the `PaneSpec` that runs the harness. The pane's `$TMUX_PANE` is how hooks find the session again. Hook names map to harness events in `domain` (`HookEvent`, `ClaudeNotification`).

- **Codex**: `internal/adapters/codex` covers the `setup codex` merge into `hooks.json`, hook and notify payload parsing, the rollout reader that supplies model, effort, context and limits, pane lookup, and launching (`codex.Adapter` is registered beside Claude's, so `session.launch --harness codex` works). The daemon applies a Codex hook's model at once and reads the rollout in a worker, never on the loop. The mapping, formulas and where each number comes from are in [internal/adapters/codex/README.md](internal/adapters/codex/README.md) and [docs/adr/0010-codex-adapter.md](docs/adr/0010-codex-adapter.md).
- **Claude** (`adapters/claude`): `agentws setup claude [--remove]` merges hooks and the status-line wrapper into Claude's `settings.json`, backs it up, and undoes it. `agentws statusline` chains the user's own status line and reports model, effort, context left and rate limits. See [docs/adr/0011-claude-harness-adapter.md](docs/adr/0011-claude-harness-adapter.md).

## Worktree setup

`agentws setup-worktree <path>` applies a repo's recipe to a new worktree. See [docs/adr/0013-setup-recipes.md](docs/adr/0013-setup-recipes.md).

- **Recipe.** The `[setup]` table of `<main checkout>/.agentws.toml`: `copy`, `link`, `run` and `deps` (`clone`, `link` or `install`). Order: copy, link, deps, run; the first failure stops. Paths already in the worktree are never overwritten.
- **Deps.** `clone` is `cp -c -R` (APFS clonefile). A lockfile that differs from main's, or a main without `node_modules`, falls back to a frozen install (`bun`, `pnpm`, `yarn` or `npm ci`, chosen by lockfile) and logs why.
- **Layers.** Rules in `domain` (`Recipe.Validate`, `PlanDeps`, `InstallCommand`). `app.WorktreeSetup` uses the `RecipeSource`, `MainCheckouts`, `SetupFS` and `CommandRunner` ports. Adapters: `adapters/setup` and `adapters/git`.
- **Process.** It runs in the CLI process, not the daemon, and reports duration and the change in free bytes on the volume.
- **Budget.** Cloning a 1 GB `node_modules` must take under 5 s and use under 50 MB. Measured on APFS with 1000 files: 0.13 s and 0.3 MB.

## Worktrees

See [docs/adr/0012-worktree-detection.md](docs/adr/0012-worktree-detection.md). `agentws worktree list` prints each worktree's path, branch, owner and PR; `agentws worktree assign <path> <session>` sets the owner.

- **Model.** A worktree's ID is its path. `Worktree.SessionID` is the owner (empty is unassigned) and the owner's `Session.WorktreeIDs` lists it. `Worktree.PR` carries number, state and the check rollup.
- **Scan.** One goroutine runs `git worktree list --porcelain -z` (`adapters/git.Worktrees`, at most 4 in flight) from every registered repo and every session's last hook `cwd`, one listing per main checkout. It runs at start, every 10 s, on `workspace.add`, and when a hook reports a new cwd or a `git worktree add`. A repo git cannot read is skipped, so its worktrees are never dropped by mistake. Prunable entries (directory gone) count as removed.
- **Attribution** (`domain.AttributeWorktree`), for worktrees not seen before: the parent session of a subagent worktree (`<cwd>/.claude/worktrees/agent-*`), then a session whose cwd is inside it, then a `git worktree add` claim from a `PostToolUse` hook in the last 30 s. With claims from several sessions, only one whose command names the path or branch wins. Otherwise unassigned.
- **Adoption.** The first scan of a repo in a daemon's life adopts its unknown worktrees as unassigned; stored worktrees keep their owner across restarts.
- **PRs.** Every 60 s, one `gh pr list --state all --json ...` per repo (`adapters/github`), matched by head branch: the open PR, else the newest. A diff goes out only when the PR or its checks changed.
- Hooks are parsed on the loop (a small JSON decode); all git and gh calls run on the scanner goroutine.

## Ports

See [docs/adr/0022-ports-view.md](docs/adr/0022-ports-view.md). Each worktree carries `Ports`: the dev servers whose cwd is inside it.

- **Read.** `adapters/procs` (`app.ProcessTable`) runs `netstat -anv -p tcp` for listening sockets, then one `lsof -a -d cwd -p <pids>` for their group, command and cwd. One goroutine does this every 5 s, and only while a worktree exists.
- **Map.** The loop maps listeners to the deepest worktree containing the cwd (`domain.PortsByWorktree`) and emits a `worktree` diff where the ports changed. Ports are never stored.
- **Kill.** `ports.kill` takes process group ids. `domain.KillGroups` keeps those that serve a listed port, minus group 1 and the daemon's own; `Terminate` sends SIGTERM to the group, then SIGKILL after 3 s. The port leaves the view on the next refresh.
- **TUI.** Ports show on worktree rows, the session's second row and the status line. `K` asks before killing the selected session's servers.
- **Budget.** A refresh (both commands) must cost under 50 ms; `BenchmarkPortsRefresh` fails above that. Measured 19 ms.

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
| Ports refresh (netstat + lsof) | < 50 ms, at most every 5 s |

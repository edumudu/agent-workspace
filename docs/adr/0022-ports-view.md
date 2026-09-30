# ADR 0022: Ports view

Status: accepted, 2026-09-30.

## Decision

- `app.ProcessTable` has two methods: `Listeners` (TCP listen sockets with pid, process group, command and cwd) and `Terminate` (end a process group). `adapters/procs` implements it.
- `Listeners` runs `netstat -anv -p tcp` for the sockets and their pids, then one `lsof -a -d cwd -p <pids> +c 0 -F pgcn` for just those processes' group, full command and cwd. `Terminate` sends SIGTERM to the whole group, waits up to 3 s for it to go, then SIGKILL. It refuses group 1 and below and its own group.
- `domain.PortsByWorktree` maps a listener to the deepest worktree whose path contains its cwd. `Worktree.Ports` carries the result. Ports are live process state, so the daemon never stores them.
- One goroutine reads the table every 5 s (`DefaultPortsPoll`) and hands the listeners to the loop, which maps them with the pure function and emits a `worktree` diff only where the ports changed. The loop keeps the last listeners, so a worktree found later gets its ports in the same turn. With no worktrees the goroutine reads nothing.
- `ports.kill` takes process group ids. `domain.KillGroups` keeps only groups that serve a port on some listed worktree, and never group 1 or below or the daemon's own. The daemon calls `Terminate` on the connection goroutine and answers with the groups that went, or `not_found` when none was allowed. It does not edit state: the next refresh drops the ports.
- The TUI shows ports on each worktree row, on the session's second row and in the status line, and `K` asks `kill :3000 :8081? y/n` for the selected session's servers; `y` sends one `ports.kill` for their distinct groups, and any other key cancels.

## Why

- Group kill takes a dev server's children (bundlers, watchers) with it, which killing the listener pid would leave running.
- Checking the request against the ports the daemon lists means a stale or forged `ports.kill` cannot signal an arbitrary process.
- `lsof -iTCP -sTCP:LISTEN` walks every process's file descriptors: 40 ms here for 7 listeners, and 73 ms as one benchmark run measured. `netstat` reads the socket table and takes about 4 ms. Restricting lsof to the listening pids costs about 8 ms. A refresh is about 19 ms and the budget is 50 ms.
- Skipping the read with no worktrees keeps an idle daemon's cost at zero. With worktrees, 19 ms every 5 s is about 0.4% of one core.

## Limits

- macOS only: `netstat -anv` has the `name:pid` column there, and the project already targets macOS.
- A process lsof cannot read (another user's) has no cwd, so its port shows on no worktree.
- Worktree paths and lsof's cwd must agree on symlinks. Git lists resolved paths and lsof reports resolved cwds, so they do.
- A killed server's port stays in the view until the next refresh, up to 5 s. A server started in a worktree shows within that interval plus one refresh.
- A server outside every known worktree, such as one run from a main checkout, shows nowhere.

## Rejected

- **One `lsof -iTCP -sTCP:LISTEN` call** (the issue's suggestion): about 2x the budget on a busy machine.
- **Killing the pid:** leaves children holding the port or the files.
- **Caching cwds by pid:** would cut the 8 ms lsof to nothing at steady state, but a reused pid could show a wrong worktree. Not needed at 0.4%.
- **`k` for kill:** `k` already moves the selection up, so kill is `K`.

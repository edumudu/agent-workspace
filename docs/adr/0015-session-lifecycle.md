# ADR 0015: Session lifecycle and the new-session dialog

Status: accepted, 2026-09-30.

## Decision

- `session.new` (`{"workspace","work_item","harness","model","effort"}`) is the one way a session starts. The TUI dialog (`n`) and `agentws new` both call it. An empty workspace means the last used one.
- The work item is parsed in `domain.ParseWorkItem`. A `linear.app/<org>/issue/<KEY-n>[/slug]` URL becomes a Linear task (the slug fills `IssueTitle`), a `github.com/<owner>/<repo>/pull/<n>` URL becomes a PR task with ref `<repo>#<n>`, and anything else, broken URLs included, is text. `Task.URL` keeps the link. A second session on the same work item joins the existing task (`domain.FindTask`).
- `domain.PlanSessionStart` decides where the session runs. An orchestration root starts at the root with no worktree. A single repo gets `git worktree add -b <slug> $AGENTWS_HOME/worktrees/<repo>/<slug> origin/<default>` (`HEAD` when the default branch is unknown). The slug is the Linear key, `<repo>-<n>` for a PR, or the first five words of the text, cut at 40 characters. A path already held by a known worktree gets `-2`, `-3`, and so on.
- `app.Sessions.Start` adds the worktree, runs the setup recipe (`app.SetupFunc`), then opens the harness pane. It stops at the first failure and never removes a worktree it added. The work item text is the agent's first prompt.
- `session.focus` swaps the session's pane into the client's main slot and focuses it. The TUI calls it on `enter` and right after a session starts. The focused session is marked read.
- `session.end` (`x`, then `y`) kills the pane and calls `Session.End()`: state `idle`, `Pane` cleared, unfocused. The session and its worktrees stay until cleanup.
- On start, the daemon checks the sessions it restored against `tmux list-panes` off the loop and ends those whose pane is gone, such as after a reboot. This replaces the `PaneBinding` stand-in from #6.
- The Codex adapter implements `app.HarnessAdapter` like Claude's, so both launch through the same path.

## Why

- One RPC keeps the dialog and the CLI identical and keeps git and tmux off the loop: the daemon reads state on the loop, runs git, setup and tmux on the connection goroutine, then commits the task, worktree, session and last-used workspace in one query.
- Clearing `Pane` on end means a later pane that tmux gives the same ID is never mistaken for the old session.

## Limits

- The worktree branches from the local `origin/<default>`; nothing fetches first, so it is as fresh as the last fetch.
- Setup recipes (#17) are not wired yet; `Run` passes a nil `SetupFunc`, so a new worktree gets no setup until that lands.

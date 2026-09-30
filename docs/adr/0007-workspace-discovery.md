# ADR 0007: Workspace discovery and repo facts

Status: accepted, 2026-09-29.

## Decision

- Discovery is filesystem-only: stat and readdir, one level deep, following symlinks. It never runs git, so `workspace.add` answers in well under the 300 ms budget for 15 repos.
- Git facts (default branch, current branch, changed files) are a separate refresh: on add, then every 30 s, off the event loop, at most 4 repos at a time. A refresh publishes only when the facts changed, so an idle daemon sends no diffs.
- The daemon does discovery and refresh on connection goroutines and workers, and commits the result to the loop through a synchronous `commit`, so a `list` right after an `add` sees it.
- `Workspace.LastUsed` is a timestamp set on every `add` and stored in the existing JSON row; "last used" is derived by taking the newest. No settings table, no migration.
- Removing a workspace is a real delete: `Store.DeleteWorkspace` and a `removed_workspace` diff. A delete is enqueued like a put, so it supersedes an earlier unflushed put of the same root.
- The daemon takes the fs and git adapters as options (`WithWorkspaces`), not as constructor arguments, so existing callers and tests keep compiling.

## Why

- Splitting discovery from facts keeps the slow, failure-prone part (git) out of the add path; a repo git cannot read keeps its last facts instead of failing the add.
- A timestamp needs no new storage and cannot disagree with the workspace list.

## Rejected

- **One `git` call per repo on add:** 15 repos would cost several hundred ms of process spawns before the user sees anything.
- **A `last_used` key in a settings table:** a second thing to keep in sync with removals.
- **fsnotify on the root for repo changes:** more moving parts than a 30 s refresh needs; revisit when worktree detection lands.

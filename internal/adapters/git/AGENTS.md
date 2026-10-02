# internal/adapters/git

The `git` CLI via exec, never a Go git library: it is the only thing that handles worktrees, sparse checkouts and user config correctly. Tests build real repos in temp dirs with `GIT_CONFIG_GLOBAL=/dev/null` so the user's git config never leaks in.

- **Repo facts** (`RepoInspector`, at most 4 repos in flight): default branch from `origin/HEAD` (empty without one), current branch (empty when detached) and changed-file count, from one `git status --porcelain=v2 --branch -z` plus one `git symbolic-ref` per repo.
- **Worktrees**: `git worktree list --porcelain -z`, at most 4 in flight (scan rules in [internal/daemon/](../../daemon/AGENTS.md)).
- **Cleanup facts**: one `git status`, the `origin/HEAD` lookup and `git merge-base --is-ancestor` per worktree, at most 4 in flight.

## Review

See [ADR 0023](../../../docs/adr/0023-review-pane.md). Diffs are parsed from `git diff` output, not computed in Go.

- **Scopes.** `last_turn` diffs from the session's newest turn snapshot, `uncommitted` from `HEAD`, `branch` from the merge base with `origin/<default>`. All end at the working tree, untracked files included, taken as a tree from a temp copy of the index (`Review`), so the real index is never touched.
- **Turns.** Snapshots live at `refs/agentws/turns/<session>/<worktree key>/<n>`, keeping only the newest per session and worktree. They never show in `git branch`.
- **Hunks** ([ADR 0028](../../../docs/adr/0028-review-comments-and-hunks.md)). Stage is `git apply --cached`, revert `git apply -R`, both of a patch rebuilt from the shown hunk. A revert first writes the patch to `<git dir>/agentws/reverted/`. Viewed marks are stored per worktree, path and blob, and reset when the file changes. Tests (`-run 'ReviewStage|ReviewRevert'`) compare against `git add -p` and `git checkout -p` in temp repos.
- **Budget.** `BenchmarkReviewOpen50Files` (`-tags integration`) fails if a cold 50-file, 3,000-line review takes over 300 ms (about 60 ms on an M3).

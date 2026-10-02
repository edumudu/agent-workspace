# ADR 0042: Resume an ended session

Status: accepted, 2026-10-01. Lifts the "an ended session cannot be brought back" limit of [ADR 0035](0035-ended-sessions.md) for sessions that still have a worktree.

## Decision

- `Session` keeps `ResumeID`, the harness's own session id from the newest hook (`session_id`; Codex notify sends `thread-id`), and `Dir`, the dir the agent was launched in. Both are stored with the session.
- `Session.Resumable()` is true for an ended session with both. `session.resume` relaunches it through `HarnessAdapter.Launch` with `LaunchRequest.Resume`: `claude --resume <id>` or `codex resume <id>`, in `Dir`, with the session's model and effort. The session keeps its ID, task and worktrees and stops being `Ended`.
- The TUI's `u` lists resumable sessions, most recently active first (`domain.ResumableSessions`), and focuses the one it resumes.

## Why

- The worktree and the harness's transcript both outlive the pane, so the work is still there; only the way back was missing.
- The newest id wins because Claude starts a new session id on `/clear`, and the conversation to return to is the latest one.
- `Dir` is recorded at launch, not taken from the hook `cwd`, because Claude finds a transcript by the project dir it started in, and the agent may `cd` elsewhere.

## Limits

- A session with no worktree is still forgotten when it ends (ADR 0035), so it cannot be resumed.
- Sessions from before this change have no `ResumeID` until a hook arrives; ones that ended before it cannot be resumed.

# ADR 0035: Ended sessions leave the sidebar and are forgotten

Status: accepted, 2026-09-30. Replaces the "the session and its worktrees stay until cleanup" part of [ADR 0015](0015-session-lifecycle.md); everything else there still holds.

## Decision

- `Session.End()` also sets `Ended`. `domain.Sidebar` leaves ended sessions out, so `x` then `y` takes the row away at once, and the footer count drops with it.
- `Session.Forgotten()` is true for an ended session with no `WorktreeIDs` left. The daemon's `SessionChanged` turns such a change into a removal: the session and its events leave the state, `Store.DeleteSession` deletes the row and its `session_events`, and subscribers get a diff with `removed_session`.
- So a session without worktrees (an orchestration root, `debug launch`) is forgotten when it ends. One with worktrees stays in the state, hidden, until its last worktree is removed. Cleanup still weighs those worktrees against the session: an ended session is idle, so it is not live.
- Every way a session ends goes through this: `session.end`, the slot watch when an agent exits by itself, and the restart check of panes that are gone.
- The session to show after the one in view ends is picked before the end is published, because the ended session's row is gone from the sidebar order afterwards.

## Why

- An ended session has no pane, so nothing on its row can be acted on. Keeping it listed made `x` look like it did nothing.
- The record stays while it has worktrees because worktree ownership and cleanup read it. Once none are left, nothing does, so it is dropped rather than kept hidden forever.

## Limits

- An ended session with no worktree cannot be brought back; start a new one on the same work item, which joins its task. One with a worktree can be resumed (ADR 0042).
- Sessions ended before this change have no `Ended` flag and stay listed until ended again.

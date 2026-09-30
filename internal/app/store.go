package app

import "github.com/giovaniif/agent-workspace/internal/domain"

// EventsPerSession is how many of a session's newest events the store keeps.
const EventsPerSession = domain.SessionEventsKept

// Snapshot is everything the daemon restores on start.
type Snapshot struct {
	Workspaces []domain.Workspace
	Tasks      []domain.Task
	Worktrees  []domain.Worktree
	Sessions   []domain.Session
	// Events are oldest first, at most EventsPerSession per session.
	Events []domain.SessionEvent
	Viewed []domain.ViewedMark
	// Drafts are ordered by ID.
	Drafts []domain.ReviewDraft
}

// Store persists daemon state. Put methods only enqueue and never block on
// disk; a later Put for the same key replaces an unflushed earlier one.
// Flush waits until everything enqueued so far is written.
type Store interface {
	PutWorkspace(domain.Workspace)
	DeleteWorkspace(root string)
	PutTask(domain.Task)
	PutWorktree(domain.Worktree)
	DeleteWorktree(id string)
	PutSession(domain.Session)
	// DeleteSession also drops the session's events.
	DeleteSession(id string)
	// PutEvent appends; unlike the Put methods it never replaces an earlier one.
	PutEvent(domain.SessionEvent)
	PutViewed(domain.ViewedMark)
	// DeleteViewed takes a ViewedMark.Key.
	DeleteViewed(key string)
	PutDraft(domain.ReviewDraft)
	Load() (Snapshot, error)
	Flush() error
	Close() error
}

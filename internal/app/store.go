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
}

// Store persists daemon state. Put methods only enqueue and never block on
// disk; a later Put for the same key replaces an unflushed earlier one.
// Flush waits until everything enqueued so far is written.
type Store interface {
	PutWorkspace(domain.Workspace)
	DeleteWorkspace(root string)
	PutTask(domain.Task)
	PutWorktree(domain.Worktree)
	PutSession(domain.Session)
	// PutEvent appends; unlike the Put methods it never replaces an earlier one.
	PutEvent(domain.SessionEvent)
	Load() (Snapshot, error)
	Flush() error
	Close() error
}

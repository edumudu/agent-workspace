package app

import "github.com/giovaniif/agent-workspace/internal/domain"

// Snapshot is everything the daemon restores on start.
type Snapshot struct {
	Workspaces []domain.Workspace
	Tasks      []domain.Task
	Worktrees  []domain.Worktree
	Sessions   []domain.Session
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
	Load() (Snapshot, error)
	Flush() error
	Close() error
}

package app

import (
	"context"
	"fmt"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// WorktreeAdder runs `git worktree add -b branch path base` in repo.
type WorktreeAdder interface {
	AddWorktree(ctx context.Context, repo, path, branch, base string) (AddedWorktree, error)
}

// AddedWorktree holds the paths as git reports them, symlinks resolved, so
// the worktree gets the same ID and repo that worktree detection gives it.
type AddedWorktree struct {
	Main string
	Path string
}

type SetupFunc func(ctx context.Context, worktree string) error

// Every Sessions method runs git or tmux, so it is for connection
// goroutines and workers, never the daemon loop.
type Sessions struct {
	Host      TerminalHost
	Worktrees WorktreeAdder
	Setup     SetupFunc
}

// NewSession is one session to start. Task must already carry its ID.
type NewSession struct {
	ID      string
	Task    domain.Task
	Plan    domain.SessionPlan
	Harness HarnessAdapter
	Name    string
	Model   string
	Effort  string
	Prompt  string
}

type Started struct {
	Session  domain.Session
	Worktree *domain.Worktree
}

// Start adds the planned worktree, runs its setup, then opens the harness
// pane in Plan.Dir. It stops at the first failure. A worktree already added
// stays on disk, since it may hold the setup's work and cleanup owns
// removal; it comes back unowned in Started.Worktree beside the error, so the
// caller can record it and a retry plans a fresh path.
func (s Sessions) Start(ctx context.Context, req NewSession) (Started, error) {
	var wt *domain.Worktree
	dir := req.Plan.Dir
	if p := req.Plan.Worktree; p != nil {
		added, err := s.Worktrees.AddWorktree(ctx, p.RepoPath, p.Path, p.Branch, p.Base)
		if err != nil {
			return Started{}, fmt.Errorf("worktree %s: %w", p.Path, err)
		}
		wt = &domain.Worktree{ID: added.Path, Repo: added.Main, Path: added.Path, Branch: p.Branch}
		dir = added.Path
		if s.Setup != nil {
			if err := s.Setup(ctx, added.Path); err != nil {
				return Started{Worktree: wt}, fmt.Errorf("setup %s: %w", added.Path, err)
			}
		}
	}
	spec := req.Harness.Launch(LaunchRequest{Name: req.Name, Dir: dir, Model: req.Model, Effort: req.Effort, Prompt: req.Prompt})
	pane, err := s.Host.Create(ctx, spec)
	if err != nil {
		return Started{Worktree: wt}, fmt.Errorf("launch %s: %w", req.Harness.Harness(), err)
	}
	session := domain.Session{
		ID:      req.ID,
		TaskID:  req.Task.ID,
		Harness: req.Harness.Harness(),
		Pane:    string(pane),
		Model:   req.Model,
		Effort:  req.Effort,
		State:   domain.StateIdle,
	}
	if wt != nil {
		wt.SessionID = req.ID
		session.WorktreeIDs = []string{wt.ID}
	}
	return Started{Session: session, Worktree: wt}, nil
}

// End leaves the session's worktrees; cleanup decides about them.
func (s Sessions) End(ctx context.Context, session domain.Session) (domain.Session, error) {
	if session.Pane != "" {
		if err := s.Host.Kill(ctx, PaneID(session.Pane)); err != nil {
			return session, err
		}
	}
	return session.End(), nil
}

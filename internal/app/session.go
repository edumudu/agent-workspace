package app

import (
	"context"
	"fmt"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// WorktreeAdder runs `git worktree add -b branch path base` in repo.
type WorktreeAdder interface {
	AddWorktree(ctx context.Context, repo, path, branch, base string) error
}

type SetupFunc func(ctx context.Context, worktree string) error

// Every Sessions method runs git or tmux, so it is for connection
// goroutines and workers, never the daemon loop.
type Sessions struct {
	Host      TerminalHost
	Worktrees WorktreeAdder
	Setup     SetupFunc
}

// NewSession is one session to start. Task must already carry its ID;
// WorktreeID is used only when Plan has a worktree.
type NewSession struct {
	ID         string
	WorktreeID string
	Task       domain.Task
	Plan       domain.SessionPlan
	Harness    HarnessAdapter
	Name       string
	Model      string
	Effort     string
	Prompt     string
}

type Started struct {
	Session  domain.Session
	Worktree *domain.Worktree
}

// Start adds the planned worktree, runs its setup, then opens the harness
// pane in Plan.Dir. It stops at the first failure. A worktree already added
// stays on disk: it may hold the setup's work, and cleanup owns removal.
func (s Sessions) Start(ctx context.Context, req NewSession) (Started, error) {
	var wt *domain.Worktree
	if p := req.Plan.Worktree; p != nil {
		if err := s.Worktrees.AddWorktree(ctx, p.RepoPath, p.Path, p.Branch, p.Base); err != nil {
			return Started{}, fmt.Errorf("worktree %s: %w", p.Path, err)
		}
		if s.Setup != nil {
			if err := s.Setup(ctx, p.Path); err != nil {
				return Started{}, fmt.Errorf("setup %s: %w", p.Path, err)
			}
		}
		wt = &domain.Worktree{ID: req.WorktreeID, Repo: p.Repo, Path: p.Path, Branch: p.Branch}
	}
	spec := req.Harness.Launch(LaunchRequest{Name: req.Name, Dir: req.Plan.Dir, Model: req.Model, Effort: req.Effort, Prompt: req.Prompt})
	pane, err := s.Host.Create(ctx, spec)
	if err != nil {
		return Started{}, fmt.Errorf("launch %s: %w", req.Harness.Harness(), err)
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

package daemon

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// turnQueue bounds the prompts waiting for a snapshot; one past it is
// dropped rather than block the loop, and its last-turn review falls back to
// the previous snapshot.
const turnQueue = 64

const reviewTimeout = 10 * time.Second

type turnJob struct {
	session string
	dirs    []string
}

type review struct {
	git      app.ReviewGit
	reviewer *app.Reviewer
	jobs     chan turnJob
}

// WithReview enables review.open and review.viewed, snapshots each session's
// worktrees when a prompt is submitted, and drops a removed worktree's
// snapshots. All git runs on workers and connection goroutines.
func WithReview(g app.ReviewGit) Option {
	return func(d *Daemon) {
		d.rv = review{git: g, reviewer: app.NewReviewer(g), jobs: make(chan turnJob, turnQueue)}
		d.st.requestTurn = func(session string, dirs []string) {
			select {
			case d.rv.jobs <- turnJob{session: session, dirs: dirs}:
			default:
			}
		}
	}
}

func (d *Daemon) snapshotTurns(ctx context.Context) {
	if d.rv.git == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-d.rv.jobs:
			for _, dir := range job.dirs {
				jctx, cancel := context.WithTimeout(ctx, reviewTimeout)
				// why: a dir git cannot read (an orchestration root, a worktree just removed) has nothing to snapshot.
				_, _ = app.SnapshotTurn(jctx, d.rv.git, job.session, dir)
				cancel()
			}
		}
	}
}

// reviewTargets are the worktrees session owns, or, when it owns none, the
// directory its hooks report, which is a checkout in a single-repo workspace.
func (s *state) reviewTargets(session string) []app.ReviewTarget {
	defaults := map[string]string{}
	for _, ws := range s.workspaces {
		for _, r := range ws.Repos {
			defaults[r.Path] = r.DefaultBranch
		}
	}
	var out []app.ReviewTarget
	for _, w := range sorted(s.worktrees) {
		if w.SessionID == session {
			out = append(out, app.ReviewTarget{Session: session, Worktree: w, DefaultBranch: defaults[w.Repo]})
		}
	}
	if cwd := s.hints.cwd[session]; len(out) == 0 && cwd != "" {
		out = append(out, app.ReviewTarget{Session: session, Worktree: domain.Worktree{ID: cwd, Repo: cwd, Path: cwd, SessionID: session}, DefaultBranch: defaults[cwd]})
	}
	return out
}

func (s *state) promptSubmitted(session string) {
	if s.requestTurn == nil {
		return
	}
	var dirs []string
	for _, t := range s.reviewTargets(session) {
		dirs = append(dirs, t.Worktree.Path)
	}
	if len(dirs) > 0 {
		s.requestTurn(session, dirs)
	}
}

func (d *Daemon) dispatchReview(req rpc.Request) (*rpc.Response, bool) {
	if d.rv.git == nil {
		return errorResponse(req.ID, rpc.CodeUnknownMethod, "unknown method "+req.Method), true
	}
	if req.Method == rpc.MethodReviewViewed {
		return d.markViewed(req)
	}
	var p rpc.ReviewParams
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Session == "" {
		return errorResponse(req.ID, rpc.CodeBadRequest, "review.open needs a session"), true
	}
	var targets []app.ReviewTarget
	var marks []domain.ViewedMark
	found := false
	ok := d.query(func(s *state) {
		if _, found = s.sessions[p.Session]; !found {
			return
		}
		ids := map[string]bool{}
		for _, t := range s.reviewTargets(p.Session) {
			if p.Worktree == "" || p.Worktree == t.Worktree.ID {
				targets = append(targets, t)
				ids[t.Worktree.ID] = true
			}
		}
		for _, m := range s.viewed {
			if ids[m.Worktree] {
				marks = append(marks, m)
			}
		}
	})
	if !found {
		return errorResponse(req.ID, rpc.CodeNotFound, "no session "+p.Session), ok
	}
	sort.Slice(marks, func(i, j int) bool { return marks[i].Key() < marks[j].Key() })
	ctx, cancel := context.WithTimeout(context.Background(), reviewTimeout)
	defer cancel()
	out := rpc.Review{Scope: p.Scope, Worktrees: d.rv.reviewer.Review(ctx, p.Scope, targets), Viewed: marks}
	if out.Worktrees == nil {
		out.Worktrees = []domain.WorktreeReview{}
	}
	return result(req.ID, out), ok
}

func (d *Daemon) markViewed(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.ViewedParams
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Mark.Worktree == "" || p.Mark.Path == "" {
		return errorResponse(req.ID, rpc.CodeBadRequest, "review.viewed needs a worktree and a path"), true
	}
	ok := d.query(func(s *state) {
		key := p.Mark.Key()
		if p.Viewed {
			s.viewed[key] = p.Mark
			s.store.PutViewed(p.Mark)
			return
		}
		delete(s.viewed, key)
		s.store.DeleteViewed(key)
	})
	return result(req.ID, struct{}{}), ok
}

// dropTurns deletes the snapshots of worktrees the scanner saw removed,
// from their repo's main checkout. Called on the scanner goroutine.
func (d *Daemon) dropTurns(ctx context.Context, removed map[string][]string) {
	if d.rv.git == nil {
		return
	}
	for main, ids := range removed {
		for _, id := range ids {
			jctx, cancel := context.WithTimeout(ctx, reviewTimeout)
			_ = app.DropTurns(jctx, d.rv.git, main, id)
			cancel()
		}
	}
}

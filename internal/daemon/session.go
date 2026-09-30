package daemon

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

type sessionDeps struct {
	worktrees    app.WorktreeAdder
	setup        app.SetupFunc
	worktreeHome string
}

// WithSessions enables session.new: worktrees are added under worktreeHome
// and setup, when set, runs in each before the agent starts. It needs
// WithHarnesses too.
func WithSessions(worktrees app.WorktreeAdder, setup app.SetupFunc, worktreeHome string) Option {
	return func(d *Daemon) {
		d.sess = sessionDeps{worktrees: worktrees, setup: setup, worktreeHome: worktreeHome}
	}
}

func (d *Daemon) sessions() app.Sessions {
	return app.Sessions{Host: d.hs.host, Worktrees: d.sess.worktrees, Setup: d.sess.setup}
}

type newSessionInput struct {
	ws      domain.Workspace
	task    domain.Task
	isNew   bool
	taken   []string
	missing string
}

// why: git, the setup recipe and tmux all run here on the connection goroutine; the loop only reads and commits.
func (d *Daemon) newSession(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.NewSessionParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "session.new params: "+err.Error()), true
	}
	adapter, ok := d.hs.adapters[domain.Harness(p.Harness)]
	if !ok || d.hs.host == nil || d.sess.worktrees == nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "no harness "+p.Harness), true
	}
	parsed := domain.ParseWorkItem(p.WorkItem)
	var in newSessionInput
	if !d.query(func(s *state) { in = s.newSessionInput(p.Workspace, parsed) }) {
		return nil, false
	}
	switch {
	case in.missing != "" && p.Workspace == "":
		return errorResponse(req.ID, rpc.CodeBadRequest, in.missing), true
	case in.missing != "":
		return errorResponse(req.ID, rpc.CodeNotFound, in.missing), true
	}
	if in.isNew {
		in.task.ID = newID()
	}
	slug := domain.TaskSlug(in.task)
	plan := domain.PlanSessionStart(in.ws, slug, d.sess.worktreeHome, in.taken)
	name := slug
	if plan.Worktree != nil {
		name = plan.Worktree.Branch
	}
	started, err := d.sessions().Start(d.ws.ctx, app.NewSession{
		ID: newID(), WorktreeID: newID(), Task: in.task, Plan: plan, Harness: adapter,
		Name: name, Model: p.Model, Effort: p.Effort, Prompt: p.WorkItem,
	})
	if err != nil {
		return errorResponse(req.ID, rpc.CodeFailed, err.Error()), true
	}
	ok = d.query(func(s *state) {
		if in.isNew {
			s.emit(TaskChanged{Task: in.task})
		}
		if started.Worktree != nil {
			s.emit(WorktreeChanged{Worktree: *started.Worktree})
		}
		s.emit(SessionChanged{Session: started.Session})
		if ws, found := s.workspaces[in.ws.Root]; found {
			ws.LastUsed = d.ws.now()
			s.emit(WorkspaceChanged{Workspace: ws})
		}
	})
	return result(req.ID, started.Session), ok
}

func (s *state) newSessionInput(root string, parsed domain.Task) newSessionInput {
	var in newSessionInput
	if root == "" {
		ws, found := domain.LastUsedWorkspace(sorted(s.workspaces))
		if !found {
			in.missing = "no workspace to start in; add one with `agentws workspace add`"
			return in
		}
		in.ws = ws
	} else {
		ws, found := s.workspaces[root]
		if !found {
			in.missing = "no workspace " + root
			return in
		}
		in.ws = ws
	}
	known, found := domain.FindTask(sorted(s.tasks), parsed)
	in.task, in.isNew = known, !found
	if !found {
		in.task = parsed
	}
	for _, w := range s.worktrees {
		in.taken = append(in.taken, w.Path)
	}
	return in
}

func (d *Daemon) sessionByRef(req rpc.Request) (domain.Session, *rpc.Response, bool) {
	var ref rpc.SessionRef
	if err := json.Unmarshal(req.Params, &ref); err != nil {
		return domain.Session{}, errorResponse(req.ID, rpc.CodeBadRequest, req.Method+" params: "+err.Error()), true
	}
	var session domain.Session
	var found bool
	if !d.query(func(s *state) { session, found = s.sessions[ref.ID] }) {
		return domain.Session{}, nil, false
	}
	if !found {
		return domain.Session{}, errorResponse(req.ID, rpc.CodeNotFound, "no session "+ref.ID), true
	}
	if d.hs.host == nil {
		return domain.Session{}, errorResponse(req.ID, rpc.CodeUnavailable, "this daemon has no terminal host"), true
	}
	return session, nil, true
}

func (d *Daemon) endSession(req rpc.Request) (*rpc.Response, bool) {
	session, resp, ok := d.sessionByRef(req)
	if resp != nil || !ok {
		return resp, ok
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()
	ended, err := d.sessions().End(ctx, session)
	if err != nil {
		return errorResponse(req.ID, rpc.CodeFailed, err.Error()), true
	}
	ok = d.query(func(s *state) {
		if cur, found := s.sessions[ended.ID]; found {
			ended = cur.End()
			s.emit(SessionChanged{Session: ended})
		}
	})
	return result(req.ID, ended), ok
}

var errNoLayout = errors.New("no client layout is open")

// focusSession makes the session the one in view. With a terminal host and a
// client layout it first swaps the session's pane into the main slot and
// focuses it; without them it only marks the session.
func (d *Daemon) focusSession(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.SessionFocusParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "session.focus params: "+err.Error()), true
	}
	var session domain.Session
	var found bool
	if !d.query(func(s *state) { session, found = s.sessions[p.ID] }) {
		return nil, false
	}
	if !found {
		return errorResponse(req.ID, rpc.CodeNotFound, "no session "+p.ID), true
	}
	if session.Pane != "" && d.hs.host != nil {
		if err := d.showInMain(app.PaneID(session.Pane)); err != nil && !errors.Is(err, errNoLayout) {
			return errorResponse(req.ID, rpc.CodeFailed, err.Error()), true
		}
	}
	ok := d.query(func(s *state) {
		if cur, found := s.sessions[p.ID]; found {
			s.focus(cur)
		}
	})
	return result(req.ID, struct{}{}), ok
}

func (d *Daemon) showInMain(pane app.PaneID) error {
	d.clients.mu.Lock()
	defer d.clients.mu.Unlock()
	if d.clients.host == nil || d.clients.slot == "" {
		return errNoLayout
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()
	if err := d.hs.host.Show(ctx, pane, d.clients.slot); err != nil {
		return err
	}
	return d.clients.host.FocusSlot(ctx, d.clients.slot)
}

// reconcilePanes ends restored sessions whose pane did not survive, such as
// after a reboot killed the tmux server. A session that changed pane while
// tmux was listed is left alone.
func (d *Daemon) reconcilePanes(ctx context.Context) {
	var onPanes []domain.Session
	for _, s := range d.restored {
		if s.Pane != "" {
			onPanes = append(onPanes, s)
		}
	}
	if d.hs.host == nil || len(onPanes) == 0 {
		return
	}
	ended, err := app.ReconcilePanes(ctx, d.hs.host, onPanes)
	if err != nil {
		return
	}
	panes := map[string]string{}
	for _, s := range onPanes {
		panes[s.ID] = s.Pane
	}
	d.query(func(s *state) {
		for _, e := range ended {
			if cur, found := s.sessions[e.ID]; found && cur.Pane == panes[e.ID] {
				s.emit(SessionChanged{Session: cur.End()})
			}
		}
	})
}

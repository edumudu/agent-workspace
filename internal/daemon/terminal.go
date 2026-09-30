package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

type terminals struct {
	mu     sync.Mutex
	home   string
	editor app.Editor
	shells map[string]app.PaneID
	nvims  map[string]app.PaneID
}

// WithTerminals enables shell.toggle, nvim.toggle and nvim.open. Shell and
// nvim panes get home as AGENTWS_HOME, so the `agentws` they run reaches this
// daemon, and nvims listen on home/nvim/<session>.sock. It needs
// WithHarnesses and a client host.
func WithTerminals(home string, editor app.Editor) Option {
	return func(d *Daemon) {
		d.term.home, d.term.editor = home, editor
		d.term.shells, d.term.nvims = map[string]app.PaneID{}, map[string]app.PaneID{}
	}
}

type terminalInput struct {
	session   domain.Session
	worktrees []domain.Worktree
	cwd       string
	found     bool
}

func (d *Daemon) terminalInput(sessionID string) (terminalInput, bool) {
	var in terminalInput
	ok := d.query(func(s *state) {
		in.session, in.found = s.sessions[sessionID]
		in.cwd = s.hints.cwd[sessionID]
		for _, w := range sorted(s.worktrees) {
			if w.SessionID == sessionID {
				in.worktrees = append(in.worktrees, w)
			}
		}
	})
	return in, ok
}

func (d *Daemon) dispatchTerminal(req rpc.Request) (*rpc.Response, bool) {
	if d.term.shells == nil || d.hs.host == nil {
		return errorResponse(req.ID, rpc.CodeUnknownMethod, "unknown method "+req.Method), true
	}
	var p struct {
		Session  string `json:"session"`
		Worktree string `json:"worktree"`
		Path     string `json:"path"`
		Line     int    `json:"line"`
		Popup    bool   `json:"popup"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, req.Method+" params: "+err.Error()), true
	}
	in, ok := d.terminalInput(p.Session)
	if !ok {
		return nil, false
	}
	if !in.found {
		return errorResponse(req.ID, rpc.CodeNotFound, "no session "+p.Session), true
	}
	target, placed := domain.ChooseShell(in.session.ID, in.worktrees, p.Worktree, in.cwd)
	if !placed {
		return errorResponse(req.ID, rpc.CodeBadRequest, "no directory known for session "+p.Session+" and worktree "+strconv.Quote(p.Worktree)), true
	}
	ctx, cancel := context.WithTimeout(context.Background(), terminalTimeout)
	defer cancel()
	d.term.mu.Lock()
	defer d.term.mu.Unlock()
	var out any
	var err error
	switch req.Method {
	case rpc.MethodShellToggle:
		out, err = d.toggleShell(ctx, in.session, target, p.Popup)
	case rpc.MethodNvimToggle:
		out, err = d.toggleNvim(ctx, in.session, target)
	default:
		out, err = d.openInNvim(ctx, in.session, target, p.Path, p.Line)
	}
	var bad badRequest
	switch {
	case errors.As(err, &bad):
		return errorResponse(req.ID, rpc.CodeBadRequest, err.Error()), true
	case err != nil:
		return errorResponse(req.ID, rpc.CodeFailed, err.Error()), true
	}
	return result(req.ID, out), true
}

const terminalTimeout = 5 * time.Second

type badRequest struct{ msg string }

func (e badRequest) Error() string { return e.msg }

// livePane returns the pane in panes[key] if it is still running, else
// starts one from spec.
func (d *Daemon) livePane(ctx context.Context, panes map[string]app.PaneID, key string, spec app.PaneSpec) (app.PaneID, bool, error) {
	if pane, ok := panes[key]; ok {
		if alive, err := d.hs.host.Alive(ctx, pane); err == nil && alive {
			return pane, false, nil
		}
		delete(panes, key)
	}
	pane, err := d.hs.host.Create(ctx, spec)
	if err != nil {
		return "", false, err
	}
	panes[key] = pane
	return pane, true, nil
}

func (d *Daemon) paneEnv(session string) map[string]string {
	return map[string]string{"AGENTWS_SESSION": session, "AGENTWS_HOME": d.term.home}
}

func (d *Daemon) withClient(f func(ctx context.Context, h ClientHost, slot app.Slot) error) error {
	d.clients.mu.Lock()
	defer d.clients.mu.Unlock()
	if d.clients.host == nil || d.clients.slot == "" {
		return errNoLayout
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()
	return f(ctx, d.clients.host, d.clients.slot)
}

func (d *Daemon) toggleShell(ctx context.Context, session domain.Session, target domain.ShellTarget, popup bool) (rpc.ShellResult, error) {
	name := "shell-" + strings.ReplaceAll(target.Key, "/", "-")
	pane, _, err := d.livePane(ctx, d.term.shells, target.Key, app.PaneSpec{Name: name, Dir: target.Dir, Env: d.paneEnv(session.ID)})
	if err != nil {
		return rpc.ShellResult{}, err
	}
	out := rpc.ShellResult{Pane: string(pane), Dir: target.Dir, Shown: true}
	if popup {
		return out, d.withClient(func(ctx context.Context, h ClientHost, _ app.Slot) error { return h.Popup(ctx, pane) })
	}
	if session.Pane != "" {
		if err := d.showInMain(app.PaneID(session.Pane)); err != nil {
			return rpc.ShellResult{}, err
		}
	}
	err = d.withClient(func(ctx context.Context, h ClientHost, slot app.Slot) error {
		if h.BelowPane(ctx, slot) == pane {
			out.Shown = false
			return h.HideBelow(ctx, slot)
		}
		return h.ShowBelow(ctx, pane, slot)
	})
	return out, err
}

func (d *Daemon) nvimSocket(session string) string {
	return filepath.Join(d.term.home, "nvim", session+".sock")
}

// nvimPane finds the session's running nvim or starts one in dir, on file at
// line when given. started says the file is already open, so no Eval is due.
func (d *Daemon) nvimPane(ctx context.Context, session domain.Session, dir, file string, line int) (pane app.PaneID, started bool, err error) {
	sock := d.nvimSocket(session.ID)
	command := []string{"nvim", "--listen", sock}
	if file != "" {
		command = append(command, "+"+strconv.Itoa(max(line, 1)), file)
	}
	spec := app.PaneSpec{Name: "nvim-" + session.ID, Dir: dir, Command: command, Env: d.paneEnv(session.ID)}
	if existing, ok := d.term.nvims[session.ID]; ok {
		if alive, aerr := d.hs.host.Alive(ctx, existing); aerr == nil && alive {
			return existing, false, nil
		}
		delete(d.term.nvims, session.ID)
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		return "", false, err
	}
	// why: a socket left by an nvim that died would make the new one fail to listen.
	_ = os.Remove(sock)
	pane, err = d.hs.host.Create(ctx, spec)
	if err != nil {
		return "", false, err
	}
	d.term.nvims[session.ID] = pane
	return pane, true, nil
}

func (d *Daemon) toggleNvim(ctx context.Context, session domain.Session, target domain.ShellTarget) (rpc.NvimResult, error) {
	pane, _, err := d.nvimPane(ctx, session, target.Dir, "", 0)
	if err != nil {
		return rpc.NvimResult{}, err
	}
	out := rpc.NvimResult{Pane: string(pane), Socket: d.nvimSocket(session.ID), Shown: true}
	err = d.withClient(func(ctx context.Context, h ClientHost, slot app.Slot) error {
		if h.ShownIn(ctx, slot) == pane {
			if session.Pane == "" {
				return errors.New("the session has no agent pane to put back")
			}
			out.Shown = false
			return d.hs.host.Show(ctx, app.PaneID(session.Pane), slot)
		}
		return d.showAndFocus(ctx, h, pane, slot)
	})
	return out, err
}

func (d *Daemon) showAndFocus(ctx context.Context, h ClientHost, pane app.PaneID, slot app.Slot) error {
	if err := d.hs.host.Show(ctx, pane, slot); err != nil {
		return err
	}
	return h.FocusSlot(ctx, slot)
}

func (d *Daemon) openInNvim(ctx context.Context, session domain.Session, target domain.ShellTarget, path string, line int) (rpc.NvimResult, error) {
	if path == "" {
		return rpc.NvimResult{}, badRequest{"nvim.open needs a path"}
	}
	file := path
	if !filepath.IsAbs(file) {
		file = filepath.Join(target.Dir, file)
	}
	file = filepath.Clean(file)
	if rel, err := filepath.Rel(target.Dir, file); err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return rpc.NvimResult{}, badRequest{path + " is outside " + target.Dir}
	}
	pane, started, err := d.nvimPane(ctx, session, target.Dir, file, line)
	if err != nil {
		return rpc.NvimResult{}, err
	}
	sock := d.nvimSocket(session.ID)
	if !started {
		if err := d.term.editor.Eval(ctx, sock, domain.NvimOpenExpr(file, line)); err != nil {
			return rpc.NvimResult{}, err
		}
	}
	err = d.withClient(func(ctx context.Context, h ClientHost, slot app.Slot) error {
		if h.ShownIn(ctx, slot) == pane {
			return h.FocusSlot(ctx, slot)
		}
		return d.showAndFocus(ctx, h, pane, slot)
	})
	return rpc.NvimResult{Pane: string(pane), Socket: sock, Shown: true}, err
}

func (d *Daemon) addComment(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.CommentParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "review.comment params: "+err.Error()), true
	}
	start, end, lines := domain.NormalizeLines(p.StartLine, p.EndLine)
	switch {
	case !lines:
		return errorResponse(req.ID, rpc.CodeBadRequest, "review.comment needs a line of 1 or more"), true
	case strings.TrimSpace(p.Body) == "":
		return errorResponse(req.ID, rpc.CodeBadRequest, "review.comment needs a comment"), true
	}
	var resp *rpc.Response
	ok := d.query(func(s *state) {
		if _, found := s.sessions[p.Session]; !found {
			resp = errorResponse(req.ID, rpc.CodeNotFound, "no session "+p.Session)
			return
		}
		worktree, path, placed := s.commentTarget(p)
		if !placed {
			resp = errorResponse(req.ID, rpc.CodeBadRequest, "the file is in none of the session's worktrees")
			return
		}
		c := domain.DraftComment{
			ID: newID(), Session: p.Session, Worktree: worktree, Path: path,
			StartLine: start, EndLine: end, Code: p.Code, Body: p.Body, At: d.ws.now(),
		}
		s.emit(CommentAdded{Comment: c})
		resp = result(req.ID, c)
	})
	return resp, ok
}

func (s *state) commentTarget(p rpc.CommentParams) (worktree, path string, ok bool) {
	var owned []domain.Worktree
	for _, w := range s.worktrees {
		if w.SessionID == p.Session {
			owned = append(owned, w)
		}
	}
	if p.File != "" {
		w, rel, found := domain.ResolveCommentFile(p.Session, owned, p.File)
		return w.ID, rel, found
	}
	for _, w := range owned {
		if w.ID == p.Worktree && p.Path != "" {
			return w.ID, p.Path, true
		}
	}
	return "", "", false
}

type CommentAdded struct{ Comment domain.DraftComment }

func (e CommentAdded) apply(s *state) rpc.Diff {
	s.comments[e.Comment.ID] = e.Comment
	return rpc.Diff{Comment: &e.Comment}
}

func (s *state) commentList() []domain.DraftComment {
	out := make([]domain.DraftComment, 0, len(s.comments))
	for _, c := range s.comments {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

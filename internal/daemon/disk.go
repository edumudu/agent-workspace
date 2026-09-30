package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const (
	// diskWorkers bounds the du processes running at once.
	diskWorkers = 2
	// diskSizeTTL is how long a measured size is trusted before it is
	// measured again in the background.
	diskSizeTTL = 5 * time.Minute
)

// recentCleanups is how many audit log lines a DiskView carries.
const recentCleanups = 8

// DiskDeps are what disk.view reads besides the cleanup plan. VolumePath is
// the directory whose volume is reported; DepsStore, when set, is the shared
// dependency store whose size is shown.
type DiskDeps struct {
	Sizes      *app.DiskSizes
	Volume     app.VolumeStat
	History    app.CleanupHistory
	VolumePath string
	DepsStore  string
}

// WithDisk serves disk.view. It needs WithCleanup: the rows are its plan.
// Sizes are only ever measured by deps.Sizes' own workers, never on the
// loop or the connection asking.
func WithDisk(deps DiskDeps) Option {
	return func(d *Daemon) { d.disk = deps }
}

func (d *Daemon) diskView(req rpc.Request) (*rpc.Response, bool) {
	if d.disk.Sizes == nil || d.cl.c == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "the disk view is not configured"), true
	}
	results, ok := d.runCleanup(d.ws.ctx, false)
	if !ok {
		return nil, false
	}
	view := rpc.DiskView{AutoCleanEvery: d.cl.every, Rows: make([]domain.DiskRow, 0, len(results)), Recent: []rpc.RecentCleanup{}}
	for _, r := range results {
		w := r.Decision.Worktree
		size, known := d.disk.Sizes.Get(w.Path)
		if !known {
			size = domain.SizePending
		}
		view.Rows = append(view.Rows, domain.DiskRow{WorktreeID: w.ID, Size: size, Action: r.Decision.Action, Reason: r.Decision.Reason})
	}
	if d.disk.Volume != nil {
		view.Free, view.Total, _ = d.disk.Volume.Stat(d.disk.VolumePath)
	}
	if d.disk.DepsStore != "" {
		size, known := d.disk.Sizes.Get(d.disk.DepsStore)
		if !known {
			size = domain.SizePending
		}
		view.DepsStore = &rpc.DepsStore{Path: d.disk.DepsStore, Size: size}
	}
	if d.disk.History != nil {
		for _, r := range d.disk.History.Recent(recentCleanups) {
			view.Recent = append(view.Recent, rpc.RecentCleanup{At: r.At, Path: r.Path, Branch: r.Branch, Action: r.Action, Outcome: r.Outcome})
		}
	}
	return result(req.ID, view), true
}

func (d *Daemon) cleanupWorktree(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.CleanupWorktreeParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "cleanup.worktree params: "+err.Error()), true
	}
	if d.cl.c == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "cleanup is not configured"), true
	}
	d.cl.mu.Lock()
	defer d.cl.mu.Unlock()
	wts, activity, ok := d.cleanupInputs()
	if !ok {
		return nil, false
	}
	for _, w := range wts {
		if w.Path != p.Path {
			continue
		}
		res := d.cl.c.RemoveWorktree(d.ws.ctx, w, activity, p.Backup)
		if res.Outcome == "removed" || strings.HasSuffix(res.Outcome, ", removed") {
			d.afterRemoval(w)
		}
		dec := res.Decision
		return result(req.ID, rpc.CleanupItem{Path: w.Path, Branch: w.Branch, Action: dec.Action, Reason: dec.Reason, Outcome: res.Outcome}), true
	}
	return errorResponse(req.ID, rpc.CodeNotFound, "no worktree "+p.Path), true
}

// afterRemoval drops the worktree from state at once, so the row does not
// wait for the next scan, and forgets its size.
func (d *Daemon) afterRemoval(w domain.Worktree) {
	if d.disk.Sizes != nil {
		d.disk.Sizes.Forget(w.Path)
	}
	d.Post(WorktreeRemoved{ID: w.ID})
	d.st.hints.wake()
}

// worktreeShell opens $SHELL (or sh) in the worktree and swaps it into the
// main slot. The pane is not a session and is left to the user to close.
func (d *Daemon) worktreeShell(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.WorktreeShellParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "worktree.shell params: "+err.Error()), true
	}
	var wt domain.Worktree
	var found bool
	if !d.query(func(s *state) { wt, found = s.worktrees[p.ID] }) {
		return nil, false
	}
	if !found {
		return errorResponse(req.ID, rpc.CodeNotFound, "no worktree "+p.ID), true
	}
	if d.hs.host == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "this daemon has no terminal host"), true
	}
	if !d.hasLayout() {
		return errorResponse(req.ID, rpc.CodeUnavailable, errNoLayout.Error()), true
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	pane, err := d.hs.host.Create(d.ws.ctx, app.PaneSpec{Name: "shell", Dir: wt.Path, Command: []string{shell}})
	if err != nil {
		return errorResponse(req.ID, rpc.CodeFailed, err.Error()), true
	}
	if err := d.showInMain(pane); err != nil && !errors.Is(err, errNoLayout) {
		return errorResponse(req.ID, rpc.CodeFailed, err.Error()), true
	}
	return result(req.ID, struct{}{}), true
}

func (d *Daemon) hasLayout() bool {
	d.clients.mu.Lock()
	defer d.clients.mu.Unlock()
	return d.clients.host != nil && d.clients.slot != ""
}

// DepsStorePath is the shared dependency store to report: $AGENTWS_DEPS_STORE
// when set, else pnpm's store if one exists in its default place. Empty means
// none, and the disk view leaves the store out.
func DepsStorePath() string {
	if p := os.Getenv("AGENTWS_DEPS_STORE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, p := range []string{
		filepath.Join(home, "Library", "pnpm", "store"),
		filepath.Join(home, ".local", "share", "pnpm", "store"),
	} {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
}

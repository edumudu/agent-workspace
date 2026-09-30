package daemon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const defaultRefreshInterval = 30 * time.Second

// Option configures a Daemon.
type Option func(*Daemon)

// WithWorkspaces enables the workspace.* methods. fs and git do IO, so they
// only ever run on connection goroutines and refresh workers, never the loop.
func WithWorkspaces(fs app.WorkspaceFS, git app.RepoInspector) Option {
	return func(d *Daemon) { d.ws.fs, d.ws.git = fs, git }
}

func WithClock(now func() time.Time) Option {
	return func(d *Daemon) { d.ws.now = now }
}

func WithRefreshInterval(every time.Duration) Option {
	return func(d *Daemon) { d.ws.refreshEvery = every }
}

type workspaces struct {
	fs           app.WorkspaceFS
	git          app.RepoInspector
	now          func() time.Time
	refreshEvery time.Duration
	ctx          context.Context
}

type WorkspaceRemoved struct{ Root string }

func (e WorkspaceRemoved) apply(s *state) rpc.Diff {
	delete(s.workspaces, e.Root)
	s.store.DeleteWorkspace(e.Root)
	return rpc.Diff{RemovedWorkspace: e.Root}
}

// workspaceMethod handles workspace.*; handled is false when workspaces are
// not enabled. ok is false when the daemon stopped.
func (d *Daemon) workspaceMethod(req rpc.Request) (resp *rpc.Response, ok, handled bool) {
	if d.ws.fs == nil {
		return nil, true, false
	}
	switch req.Method {
	case rpc.MethodWorkspaceAdd:
		resp, ok = d.workspaceAdd(req)
	case rpc.MethodWorkspaceRemove:
		resp, ok = d.workspaceRemove(req)
	default:
		resp, ok = d.workspaceList(req)
	}
	return resp, ok, true
}

func (d *Daemon) workspaceAdd(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.WorkspaceAddParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "params must be {\"path\": string}"), true
	}
	if !filepath.IsAbs(p.Path) {
		return errorResponse(req.ID, rpc.CodeBadRequest, "path must be absolute: "+p.Path), true
	}
	root := filepath.Clean(p.Path)
	var known []domain.Repo
	if !d.query(func(s *state) { known = s.workspaces[root].Repos }) {
		return nil, false
	}
	ws, err := app.DiscoverWorkspace(d.ws.fs, root, known)
	if err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, err.Error()), true
	}
	ws.LastUsed = d.ws.now()
	if !d.commit(WorkspaceChanged{Workspace: ws}) {
		return nil, false
	}
	go d.refreshWorkspace(root)
	d.st.hints.wake()
	return result(req.ID, ws), true
}

func (d *Daemon) workspaceRemove(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.WorkspaceRemoveParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "params must be {\"root\": string}"), true
	}
	var found bool
	ok := d.query(func(s *state) {
		if _, found = s.workspaces[p.Root]; found {
			d.st.emit(WorkspaceRemoved{Root: p.Root})
		}
	})
	if !found {
		return errorResponse(req.ID, rpc.CodeNotFound, "no workspace "+p.Root), ok
	}
	return result(req.ID, struct{}{}), ok
}

func (d *Daemon) workspaceList(req rpc.Request) (*rpc.Response, bool) {
	var list rpc.WorkspaceList
	ok := d.query(func(s *state) {
		list.Workspaces = sorted(s.workspaces)
		if last, found := domain.LastUsedWorkspace(list.Workspaces); found {
			list.LastUsed = last.Root
		}
	})
	if list.Workspaces == nil {
		list.Workspaces = []domain.Workspace{}
	}
	return result(req.ID, list), ok
}

// refreshWorkspace re-reads git facts for root off the loop and publishes
// them only if they changed and the workspace is still registered.
func (d *Daemon) refreshWorkspace(root string) {
	var ws domain.Workspace
	var found bool
	if !d.query(func(s *state) { ws, found = s.workspaces[root] }) || !found {
		return
	}
	fresh := app.RefreshRepoFacts(d.ws.ctx, d.ws.git, ws)
	d.query(func(s *state) {
		cur, found := s.workspaces[root]
		if !found {
			return
		}
		// why: the workspace may have been re-added while git ran, so carry the facts onto its current repo list.
		merged := cur
		merged.Repos = domain.MergeRepoState(fresh.Repos, cur.Repos)
		if !reflect.DeepEqual(merged, cur) {
			d.st.emit(WorkspaceChanged{Workspace: merged})
		}
	})
}

func (d *Daemon) refreshWorkspacesEvery(ctx context.Context) {
	if d.ws.fs == nil || d.ws.refreshEvery <= 0 {
		return
	}
	tick := time.NewTicker(d.ws.refreshEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			var roots []string
			d.query(func(s *state) {
				for r := range s.workspaces {
					roots = append(roots, r)
				}
			})
			for _, r := range roots {
				d.refreshWorkspace(r)
			}
		}
	}
}

package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"slices"

	"github.com/BurntSushi/toml"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

type launcherCfg struct {
	enabled     bool
	maxParallel int
	kick        chan struct{}
}

func WithLauncher(maxParallel int) Option {
	return func(d *Daemon) {
		d.lc.enabled = true
		d.lc.maxParallel = maxParallel
	}
}

func LoadMaxParallel(path string) (int, error) {
	var cfg struct {
		Launcher struct {
			MaxParallel int `toml:"max_parallel"`
		} `toml:"launcher"`
	}
	_, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("launcher: %s: %w", path, err)
	}
	return cfg.Launcher.MaxParallel, nil
}

type QueueChanged struct{ Queue []domain.LaunchItem }

func (e QueueChanged) apply(s *state) rpc.Diff {
	s.queue = e.Queue
	queue := slices.Clone(e.Queue)
	return rpc.Diff{Queue: &queue}
}

func (d *Daemon) kickLauncher() {
	select {
	case d.lc.kick <- struct{}{}:
	default:
	}
}

func (d *Daemon) runLauncher(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.lc.kick:
			d.drainLauncher()
		}
	}
}

func (d *Daemon) drainLauncher() {
	var start []domain.LaunchItem
	d.query(func(s *state) {
		if len(s.queue) == 0 {
			return
		}
		active := domain.ActiveLaunched(s.sessionList(), s.launched)
		next, started := domain.DrainLauncher(s.queue, active, d.lc.maxParallel)
		if len(started) == 0 {
			return
		}
		start = started
		s.emit(QueueChanged{Queue: next})
	})
	for _, item := range start {
		go d.startQueued(item)
	}
}

func (d *Daemon) startQueued(item domain.LaunchItem) {
	session, rerr, ok := d.startSession(rpc.NewSessionParams{
		Workspace: item.Workspace, WorkItem: item.URL,
		Harness: string(item.Request.Harness), Model: item.Request.Model, Effort: item.Request.Effort,
	}, item.URL)
	if !ok {
		return
	}
	if rerr != nil {
		log.Printf("launcher: %s: %s", item.Ref, rerr.Message)
	}
	d.query(func(s *state) {
		queue := slices.Clone(s.queue)
		i := slices.IndexFunc(queue, func(x domain.LaunchItem) bool { return x.ID == item.ID })
		switch {
		case rerr == nil:
			s.launched[session.ID] = true
			if i >= 0 {
				queue = slices.Delete(queue, i, i+1)
			}
		case i >= 0:
			queue[i].Starting, queue[i].Err = false, rerr.Message
		}
		s.emit(QueueChanged{Queue: queue})
	})
	d.kickLauncher()
}

// why: a failed item does not count, so enqueuing its issue again retries it.
func (s *state) launcherHas(ref string) bool {
	for _, i := range s.queue {
		if i.Ref == ref && i.Err == "" {
			return true
		}
	}
	for _, x := range s.sessions {
		if !s.launched[x.ID] || x.Pane == "" || x.State == domain.StateDone {
			continue
		}
		if t, ok := s.tasks[x.TaskID]; ok && t.Source == domain.TaskLinear && t.Ref == ref {
			return true
		}
	}
	return false
}

func (d *Daemon) launcherMethod(req rpc.Request) (*rpc.Response, bool) {
	if !d.lc.enabled || d.hs.host == nil || d.sess.worktrees == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "this daemon has no launcher"), true
	}
	switch req.Method {
	case rpc.MethodLauncherEnqueue:
		return d.launcherEnqueue(req)
	case rpc.MethodLauncherDrop:
		return d.launcherEdit(req, func(queue []domain.LaunchItem, raw json.RawMessage) ([]domain.LaunchItem, bool, error) {
			var ref rpc.LauncherItemRef
			if err := json.Unmarshal(raw, &ref); err != nil {
				return nil, false, err
			}
			next, ok := domain.Drop(queue, ref.ID)
			return next, ok, nil
		})
	default:
		return d.launcherEdit(req, func(queue []domain.LaunchItem, raw json.RawMessage) ([]domain.LaunchItem, bool, error) {
			var p rpc.LauncherRetargetParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, false, err
			}
			if _, known := d.hs.adapters[domain.Harness(p.Harness)]; !known {
				return nil, false, fmt.Errorf("no harness %s", p.Harness)
			}
			next, ok := domain.Retarget(queue, p.ID, domain.StartRequest{Harness: domain.Harness(p.Harness), Model: p.Model, Effort: p.Effort})
			return next, ok, nil
		})
	}
}

func (d *Daemon) launcherEnqueue(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.LauncherEnqueueParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "launcher.enqueue params: "+err.Error()), true
	}
	if _, known := d.hs.adapters[domain.Harness(p.Harness)]; !known {
		return errorResponse(req.ID, rpc.CodeBadRequest, "no harness "+p.Harness), true
	}
	issues, rejected := domain.ParseIssueURLs(p.Input)
	if len(issues) == 0 {
		return errorResponse(req.ID, rpc.CodeBadRequest, "no Linear issue URL in the input"), true
	}
	out := rpc.LauncherEnqueued{Queued: []string{}, Rejected: append([]string{}, rejected...)}
	ok := d.query(func(s *state) {
		queue := slices.DeleteFunc(slices.Clone(s.queue), func(i domain.LaunchItem) bool {
			return i.Err != "" && slices.ContainsFunc(issues, func(t domain.Task) bool { return t.Ref == i.Ref })
		})
		for _, issue := range issues {
			if s.launcherHas(issue.Ref) {
				continue
			}
			queue = append(queue, domain.LaunchItem{
				ID: newID(), Ref: issue.Ref, URL: issue.URL, Workspace: p.Workspace,
				Request: domain.StartRequest{Harness: domain.Harness(p.Harness), Model: p.Model, Effort: p.Effort},
			})
			out.Queued = append(out.Queued, issue.Ref)
		}
		s.emit(QueueChanged{Queue: queue})
	})
	d.kickLauncher()
	return result(req.ID, out), ok
}

func (d *Daemon) launcherEdit(req rpc.Request, edit func([]domain.LaunchItem, json.RawMessage) ([]domain.LaunchItem, bool, error)) (*rpc.Response, bool) {
	var editErr error
	applied := false
	ok := d.query(func(s *state) {
		next, done, err := edit(s.queue, req.Params)
		if err != nil {
			editErr = err
			return
		}
		if done {
			applied = true
			s.emit(QueueChanged{Queue: next})
		}
	})
	switch {
	case editErr != nil:
		return errorResponse(req.ID, rpc.CodeBadRequest, req.Method+": "+editErr.Error()), ok
	case ok && !applied:
		return errorResponse(req.ID, rpc.CodeNotFound, "no waiting issue for "+req.Method), true
	}
	return result(req.ID, struct{}{}), ok
}

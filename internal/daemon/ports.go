package daemon

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// DefaultPortsPoll is the most often the process table is read.
const DefaultPortsPoll = 5 * time.Second

type portScanner struct {
	table app.ProcessTable
	every time.Duration
}

// WithProcessTable enables ports on worktrees and ports.kill. Reading the
// table runs commands, so it happens only on the ports goroutine, and kills
// only on the connection that asked.
func WithProcessTable(table app.ProcessTable) Option {
	return func(d *Daemon) { d.ports.table = table }
}

func WithPortsPoll(every time.Duration) Option {
	return func(d *Daemon) { d.ports.every = every }
}

func (d *Daemon) watchPorts(ctx context.Context) {
	if d.ports.table == nil {
		return
	}
	tick, stop := ticker(d.ports.every)
	defer stop()
	for {
		d.refreshPorts(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}

// refreshPorts reads the process table and maps the listeners on the loop. It
// does nothing while there is no worktree to map to, which keeps an idle
// daemon from running lsof every few seconds. A failed read keeps the ports
// already shown.
func (d *Daemon) refreshPorts(ctx context.Context) {
	hasWorktrees := false
	if !d.query(func(s *state) { hasWorktrees = len(s.worktrees) > 0 }) || !hasWorktrees {
		return
	}
	listeners, err := d.ports.table.Listeners(ctx)
	if err != nil {
		return
	}
	d.query(func(s *state) {
		s.listeners = listeners
		s.applyPorts()
	})
}

// portsOf is w's ports given the last listeners read.
func (s *state) portsOf(w domain.Worktree) []domain.Port {
	all := sorted(s.worktrees)
	if _, known := s.worktrees[w.ID]; !known {
		all = append(all, w)
	}
	return domain.PortsByWorktree(all, s.listeners)[w.ID]
}

func (s *state) applyPorts() {
	mapped := domain.PortsByWorktree(sorted(s.worktrees), s.listeners)
	for _, w := range sorted(s.worktrees) {
		next := mapped[w.ID]
		if len(next) == 0 && len(w.Ports) == 0 || reflect.DeepEqual(next, w.Ports) {
			continue
		}
		w.Ports = next
		s.emit(WorktreeChanged{Worktree: w})
	}
}

func (d *Daemon) portsKill(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.PortsKillParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "ports.kill params must be {\"pgids\": [int]}"), true
	}
	if d.ports.table == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "no process table"), true
	}
	var groups []int
	if !d.query(func(s *state) { groups = domain.KillGroups(sorted(s.worktrees), p.PGIDs, syscall.Getpgrp()) }) {
		return nil, false
	}
	if len(groups) == 0 {
		return errorResponse(req.ID, rpc.CodeNotFound, "no listed port belongs to those process groups"), true
	}
	killed, err := d.terminate(groups)
	if len(killed) == 0 {
		return errorResponse(req.ID, rpc.CodeFailed, err.Error()), true
	}
	return result(req.ID, rpc.PortsKilled{Killed: killed}), true
}

// terminate ends the groups in parallel and returns those that went, in
// order, with the first failure.
func (d *Daemon) terminate(groups []int) ([]int, error) {
	errs := make([]error, len(groups))
	var wg sync.WaitGroup
	for i, g := range groups {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = d.ports.table.Terminate(d.ws.ctx, g)
		}()
	}
	wg.Wait()
	var killed []int
	var first error
	for i, g := range groups {
		if errs[i] == nil {
			killed = append(killed, g)
		} else if first == nil {
			first = errs[i]
		}
	}
	return killed, first
}

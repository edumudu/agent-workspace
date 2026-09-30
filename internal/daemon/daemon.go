// Package daemon owns the in-memory state and serves it over the RPC socket.
// One goroutine, the loop, owns the state: adapters Post events to it, and
// connections send it queries. Nothing in the loop touches disk or runs
// commands; the store only enqueues writes.
package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// outBuffer is how many messages a connection may fall behind before the
// daemon drops it rather than stall the loop.
const outBuffer = 1024

// Event is a state change an adapter posts to the loop.
type Event interface {
	apply(s *state) rpc.Diff
}

type WorkspaceChanged struct{ Workspace domain.Workspace }
type TaskChanged struct{ Task domain.Task }
type WorktreeChanged struct{ Worktree domain.Worktree }
type SessionChanged struct{ Session domain.Session }

func (e WorkspaceChanged) apply(s *state) rpc.Diff {
	s.workspaces[e.Workspace.Root] = e.Workspace
	s.store.PutWorkspace(e.Workspace)
	return rpc.Diff{Workspace: &e.Workspace}
}

func (e TaskChanged) apply(s *state) rpc.Diff {
	s.tasks[e.Task.ID] = e.Task
	s.store.PutTask(e.Task)
	return rpc.Diff{Task: &e.Task}
}

func (e WorktreeChanged) apply(s *state) rpc.Diff {
	s.worktrees[e.Worktree.ID] = e.Worktree
	s.store.PutWorktree(e.Worktree)
	return rpc.Diff{Worktree: &e.Worktree}
}

func (e SessionChanged) apply(s *state) rpc.Diff {
	s.sessions[e.Session.ID] = e.Session
	s.store.PutSession(e.Session)
	return rpc.Diff{Session: &e.Session}
}

type state struct {
	store      app.Store
	seq        uint64
	workspaces map[string]domain.Workspace
	tasks      map[string]domain.Task
	worktrees  map[string]domain.Worktree
	sessions   map[string]domain.Session
	subs       map[*conn]uint64
}

type Daemon struct {
	pid     int
	started time.Time
	events  chan Event
	queries chan func(*state)
	stopped chan struct{}
	st      *state
}

// New restores state from store. pid is what status reports.
func New(store app.Store, pid int) (*Daemon, error) {
	snap, err := store.Load()
	if err != nil {
		return nil, err
	}
	st := &state{
		store:      store,
		workspaces: map[string]domain.Workspace{},
		tasks:      map[string]domain.Task{},
		worktrees:  map[string]domain.Worktree{},
		sessions:   map[string]domain.Session{},
		subs:       map[*conn]uint64{},
	}
	for _, w := range snap.Workspaces {
		st.workspaces[w.Root] = w
	}
	for _, t := range snap.Tasks {
		st.tasks[t.ID] = t
	}
	for _, w := range snap.Worktrees {
		st.worktrees[w.ID] = w
	}
	for _, x := range snap.Sessions {
		st.sessions[x.ID] = x
	}
	return &Daemon{
		pid:     pid,
		started: time.Now(),
		events:  make(chan Event, 256),
		queries: make(chan func(*state)),
		stopped: make(chan struct{}),
		st:      st,
	}, nil
}

// Post hands an event to the loop. It blocks only while the loop's queue is
// full, and returns at once after Serve has stopped.
func (d *Daemon) Post(e Event) {
	select {
	case d.events <- e:
	case <-d.stopped:
	}
}

// query runs f on the loop and waits for it.
func (d *Daemon) query(f func(*state)) bool {
	done := make(chan struct{})
	select {
	case d.queries <- func(s *state) { f(s); close(done) }:
	case <-d.stopped:
		return false
	}
	<-done
	return true
}

// Serve runs the loop and accepts connections on ln until ctx is done, then
// closes every connection and flushes the store.
func (d *Daemon) Serve(ctx context.Context, ln net.Listener) error {
	loopDone := make(chan struct{})
	go func() {
		d.loop(ctx)
		close(loopDone)
	}()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	var conns sync.WaitGroup
	var mu sync.Mutex
	open := map[*conn]struct{}{}
	for {
		nc, err := ln.Accept()
		if err != nil {
			break
		}
		c := newConn(nc)
		mu.Lock()
		open[c] = struct{}{}
		mu.Unlock()
		conns.Add(1)
		go func() {
			defer conns.Done()
			d.handle(c)
			mu.Lock()
			delete(open, c)
			mu.Unlock()
		}()
	}
	<-loopDone
	mu.Lock()
	for c := range open {
		c.kill()
	}
	mu.Unlock()
	conns.Wait()
	return d.st.store.Flush()
}

func (d *Daemon) loop(ctx context.Context) {
	defer close(d.stopped)
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-d.events:
			d.st.seq++
			diff := e.apply(d.st)
			diff.Seq = d.st.seq
			for c, id := range d.st.subs {
				if !c.push(rpc.Response{V: rpc.Version, ID: id, Diff: &diff}) {
					delete(d.st.subs, c)
				}
			}
		case q := <-d.queries:
			q(d.st)
		}
	}
}

func (d *Daemon) handle(c *conn) {
	defer func() {
		c.kill()
		d.query(func(s *state) { delete(s.subs, c) })
	}()
	go c.write()
	sc := bufio.NewScanner(c.nc)
	sc.Buffer(make([]byte, 64*1024), rpc.MaxMessage)
	for sc.Scan() {
		resp, ok := d.dispatch(c, sc.Bytes())
		if !ok {
			return
		}
		if resp != nil && !c.push(*resp) {
			return
		}
	}
}

// dispatch answers one request line. A nil response means the loop already
// replied, as subscribe does.
func (d *Daemon) dispatch(c *conn, line []byte) (*rpc.Response, bool) {
	var req rpc.Request
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(0, rpc.CodeBadRequest, "request is not a JSON object: "+err.Error()), true
	}
	if req.V != rpc.Version {
		return errorResponse(req.ID, rpc.CodeUnsupportedVersion, "this daemon speaks protocol v1"), true
	}
	switch req.Method {
	case rpc.MethodStatus:
		var st rpc.Status
		ok := d.query(func(s *state) {
			st = rpc.Status{PID: d.pid, StartedAt: d.started, Sessions: len(s.sessions), Worktrees: len(s.worktrees)}
		})
		return result(req.ID, st), ok
	case rpc.MethodSubscribe:
		ok := d.query(func(s *state) {
			if c.push(*result(req.ID, s.snapshot())) {
				s.subs[c] = req.ID
			}
		})
		return nil, ok
	default:
		return errorResponse(req.ID, rpc.CodeUnknownMethod, "unknown method "+req.Method), true
	}
}

func (s *state) snapshot() rpc.State {
	return rpc.State{
		Seq:        s.seq,
		Workspaces: sorted(s.workspaces),
		Tasks:      sorted(s.tasks),
		Worktrees:  sorted(s.worktrees),
		Sessions:   sorted(s.sessions),
	}
}

func sorted[T any](m map[string]T) []T {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]T, 0, len(m))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func result(id uint64, v any) *rpc.Response {
	b, err := json.Marshal(v)
	if err != nil {
		return errorResponse(id, rpc.CodeBadRequest, err.Error())
	}
	return &rpc.Response{V: rpc.Version, ID: id, Result: b}
}

func errorResponse(id uint64, code, msg string) *rpc.Response {
	return &rpc.Response{V: rpc.Version, ID: id, Error: &rpc.Error{Code: code, Message: msg}}
}

type conn struct {
	nc   net.Conn
	out  chan rpc.Response
	gone chan struct{}
	once sync.Once
}

func newConn(nc net.Conn) *conn {
	return &conn{nc: nc, out: make(chan rpc.Response, outBuffer), gone: make(chan struct{})}
}

// push queues resp without blocking. A connection too far behind is dropped,
// and push reports false.
func (c *conn) push(resp rpc.Response) bool {
	select {
	case <-c.gone:
		return false
	default:
	}
	select {
	case c.out <- resp:
		return true
	default:
		c.kill()
		return false
	}
}

func (c *conn) kill() {
	c.once.Do(func() {
		close(c.gone)
		_ = c.nc.Close()
	})
}

func (c *conn) write() {
	enc := json.NewEncoder(c.nc)
	for {
		select {
		case resp := <-c.out:
			if err := enc.Encode(resp); err != nil {
				c.kill()
				return
			}
		case <-c.gone:
			return
		}
	}
}

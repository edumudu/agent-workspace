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

	"github.com/giovaniif/agent-workspace/internal/adapters/codex"
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

// SessionHooked is a session change together with the hook event that caused
// it, published as one diff so a subscriber never sees one without the other.
type SessionHooked struct {
	Session domain.Session
	Event   domain.SessionEvent
}

type SubagentChanged struct{ Subagent domain.Subagent }

func (e SubagentChanged) apply(s *state) rpc.Diff {
	var merged domain.Subagent
	s.subagents, merged = domain.TrackSubagent(s.subagents, e.Subagent)
	return rpc.Diff{Subagent: &merged}
}

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
	stored := e.Worktree
	stored.Ports = nil
	s.store.PutWorktree(stored)
	return rpc.Diff{Worktree: &e.Worktree}
}

func (e SessionChanged) apply(s *state) rpc.Diff {
	s.sessions[e.Session.ID] = e.Session
	s.store.PutSession(e.Session)
	return rpc.Diff{Session: &e.Session}
}

func (e SessionHooked) apply(s *state) rpc.Diff {
	s.sessions[e.Session.ID] = e.Session
	s.store.PutSession(e.Session)
	kept := append(s.events[e.Event.SessionID], e.Event)
	if len(kept) > app.EventsPerSession {
		kept = kept[len(kept)-app.EventsPerSession:]
	}
	s.events[e.Event.SessionID] = kept
	s.store.PutEvent(e.Event)
	return rpc.Diff{Session: &e.Session, Event: &e.Event}
}

type state struct {
	store      app.Store
	seq        uint64
	workspaces map[string]domain.Workspace
	tasks      map[string]domain.Task
	worktrees  map[string]domain.Worktree
	sessions   map[string]domain.Session
	events     map[string][]domain.SessionEvent
	subagents  []domain.Subagent
	subs       map[*conn]uint64
	usage      map[string]*usageJob
	attn       *attention
	// requestUsage is set by New; state cannot reach the Daemon that owns it.
	requestUsage func(sessionID, path string, force bool)
	// sendSwitches is set by New for the same reason.
	sendSwitches func(session domain.Session, sws []domain.Switch)
	hints        worktreeHints
	listeners    []domain.Listener
	// requestTurn is set by WithReview and must not block.
	requestTurn func(session string, dirs []string)
	viewed      map[string]domain.ViewedMark
}

type Daemon struct {
	pid     int
	started time.Time
	events  chan Event
	queries chan func(*state)
	stopped chan struct{}
	st      *state
	ws      workspaces
	clients clients
	hs      harnesses
	wt      worktreeScanner
	ports   portScanner
	sess    sessionDeps
	// restored is the sessions loaded from the store, checked once against tmux on Serve.
	restored []domain.Session
	rv       review
	cl       cleanupWorker
}

// New restores state from store. pid is what status reports.
func New(store app.Store, pid int, opts ...Option) (*Daemon, error) {
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
		events:     map[string][]domain.SessionEvent{},
		subs:       map[*conn]uint64{},
		usage:      map[string]*usageJob{},
		hints:      newWorktreeHints(),
		viewed:     map[string]domain.ViewedMark{},
	}
	for _, m := range snap.Viewed {
		st.viewed[m.Key()] = m
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
		// why: nobody is looking at a session across a daemon restart, and a stale flag would hide its banners.
		st.sessions[x.ID] = x.Blur()
	}
	for _, ev := range snap.Events {
		st.events[ev.SessionID] = append(st.events[ev.SessionID], ev)
	}
	d := &Daemon{
		pid:      pid,
		started:  time.Now(),
		events:   make(chan Event, 256),
		queries:  make(chan func(*state)),
		stopped:  make(chan struct{}),
		st:       st,
		restored: snap.Sessions,
		ws:       workspaces{now: time.Now, refreshEvery: defaultRefreshInterval, ctx: context.Background()},
		wt:       worktreeScanner{every: DefaultWorktreePoll, prEvery: DefaultPRPoll, baselined: map[string]bool{}},
		ports:    portScanner{every: DefaultPortsPoll},
		cl:       cleanupWorker{kick: make(chan struct{}, 1)},
	}
	st.requestUsage = func(sessionID, path string, force bool) { d.requestUsage(st, sessionID, path, force) }
	st.sendSwitches = d.sendSwitches
	for _, o := range opts {
		o(d)
	}
	return d, nil
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
	d.ws.ctx = ctx
	go d.refreshWorkspacesEvery(ctx)
	if d.st.attn != nil {
		go d.st.attn.run(ctx)
	}
	go d.watchWorktrees(ctx)
	go d.watchPorts(ctx)
	go d.reconcilePanes(ctx)
	go d.watchMainSlot(ctx)
	go d.snapshotTurns(ctx)
	go d.cleanupEvery(ctx)
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
			d.st.emit(e)
		case q := <-d.queries:
			q(d.st)
		}
	}
}

func (s *state) emit(e Event) {
	s.seq++
	diff := e.apply(s)
	diff.Seq = s.seq
	for c, id := range s.subs {
		if !c.push(rpc.Response{V: rpc.Version, ID: id, Diff: &diff}) {
			delete(s.subs, c)
		}
	}
}

// hook applies a harness hook to the session on its pane. Hooks from panes
// no session owns, and hook names the harness adapter does not know, are
// ignored.
func (s *state) hook(h rpc.Hook, now time.Time) {
	kind, ok := hookEvent(h)
	if !ok {
		return
	}
	session, ok := domain.SessionOnPane(s.sessionList(), h.Pane)
	if !ok {
		return
	}
	s.noteHook(session.ID, kind, h.Payload, now)
	next, effects := session.Apply(domain.HarnessEvent{Kind: kind})
	if domain.Harness(h.Harness) == domain.HarnessCodex {
		next = s.codexObservation(h, kind, session, next)
	}
	at := h.At
	if at.IsZero() {
		at = time.Now()
	}
	next, toSend := next.Dispatch(time.Now())
	ev := domain.SessionEventFromHook(kind, at, h.Payload)
	ev.SessionID = session.ID
	s.emit(SessionHooked{Session: next, Event: ev})
	s.trackSubagents(session.ID, kind, at, h.Payload)
	s.announce(next, effects)
	s.sendSwitches(next, toSend)
	if kind == domain.EventUserPromptSubmit {
		s.promptSubmitted(session.ID)
	}
}

// trackSubagents follows a subagent start or stop, and stops what a session
// spawned when it starts over or ends.
func (s *state) trackSubagents(sessionID string, kind domain.HarnessEventKind, at time.Time, payload []byte) {
	switch kind {
	case domain.EventSubagentStart, domain.EventSubagentStop:
		sub, ok := domain.SubagentFromHook(kind, at, payload)
		if !ok {
			return
		}
		sub.SessionID = sessionID
		s.emit(SubagentChanged{Subagent: sub})
	case domain.EventSessionStart, domain.EventSessionEnd:
		var changed []domain.Subagent
		s.subagents, changed = domain.EndSubagents(s.subagents, sessionID, at)
		for _, sub := range changed {
			s.emit(SubagentChanged{Subagent: sub})
		}
	}
}

// commit applies e on the loop and returns once it has taken effect, so a
// query issued afterwards sees it. It reports false if the daemon stopped.
func (d *Daemon) commit(e Event) bool {
	return d.query(func(s *state) { s.emit(e) })
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
	case rpc.MethodHook:
		var h rpc.Hook
		if err := json.Unmarshal(req.Params, &h); err != nil {
			return errorResponse(req.ID, rpc.CodeBadRequest, "hook params: "+err.Error()), true
		}
		ok := d.query(func(s *state) { s.hook(h, d.ws.now()) })
		return result(req.ID, rpc.HookReply{}), ok
	case rpc.MethodStatusLine:
		var sl rpc.StatusLine
		if err := json.Unmarshal(req.Params, &sl); err != nil {
			return errorResponse(req.ID, rpc.CodeBadRequest, "statusline params: "+err.Error()), true
		}
		ok := d.query(func(s *state) { s.statusLine(sl) })
		return result(req.ID, struct{}{}), ok
	case rpc.MethodLaunch:
		return d.launch(req)
	case rpc.MethodSessionMute:
		return d.dispatchAttention(req)
	case rpc.MethodSessionFocus:
		return d.focusSession(req)
	case rpc.MethodNewSession:
		return d.newSession(req)
	case rpc.MethodEndSession:
		return d.endSession(req)
	case rpc.MethodSwitch:
		return d.switchSession(req)
	case rpc.MethodWorkspaceAdd, rpc.MethodWorkspaceList, rpc.MethodWorkspaceRemove:
		if resp, ok, handled := d.workspaceMethod(req); handled {
			return resp, ok
		}
		return errorResponse(req.ID, rpc.CodeUnknownMethod, "unknown method "+req.Method), true
	case rpc.MethodReviewOpen, rpc.MethodReviewViewed:
		return d.dispatchReview(req)
	case rpc.MethodOpenClient, rpc.MethodFocusMain, rpc.MethodClientReview:
		return d.dispatchClient(req), true
	case rpc.MethodDebugSeed:
		var p rpc.DebugSeedParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Count < 1 {
			return errorResponse(req.ID, rpc.CodeBadRequest, "debug.seed needs a count of at least 1"), true
		}
		ok := d.query(func(s *state) {
			for _, e := range seed(p.Count) {
				s.emit(e)
			}
		})
		return result(req.ID, struct{}{}), ok
	case rpc.MethodWorktreeAssign:
		return d.worktreeAssign(req)
	case rpc.MethodPortsKill:
		return d.portsKill(req)
	case rpc.MethodCleanupPlan, rpc.MethodCleanupRun:
		return d.cleanupMethod(req)
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
		Events:     flatten(s.events),
		Subagents:  append([]domain.Subagent{}, s.subagents...),
	}
}

func flatten(bySession map[string][]domain.SessionEvent) []domain.SessionEvent {
	var out []domain.SessionEvent
	for _, events := range sorted(bySession) {
		out = append(out, events...)
	}
	return out
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

// codexObservation takes the model Codex reports at once, and asks for a
// read of the rollout, which has the effort and usage the hook lacks.
func (s *state) codexObservation(h rpc.Hook, kind domain.HarnessEventKind, before, next domain.Session) domain.Session {
	obs, err := codex.ParseHook(h.Event, h.Payload)
	if err != nil {
		return next
	}
	if obs.Model != "" {
		next.Model = obs.Model
	}
	if obs.TranscriptPath != "" {
		force := kind == domain.EventStop || kind == domain.EventSessionStart || kind == domain.EventUserPromptSubmit
		s.requestUsage(before.ID, obs.TranscriptPath, force)
	}
	return next
}

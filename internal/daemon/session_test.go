package daemon_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/adapters/claude"
	"github.com/giovaniif/agent-workspace/internal/adapters/codex"
	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/daemon"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

var (
	singleWS = domain.Workspace{Root: "/src/api", Kind: domain.WorkspaceSingle,
		Repos: []domain.Repo{{Name: "api", Path: "/src/api", DefaultBranch: "main"}}}
	orchWS = domain.Workspace{Root: "/src/shop", Kind: domain.WorkspaceOrchestration,
		Repos: []domain.Repo{{Name: "web", Path: "/src/shop/web"}}, LastUsed: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
)

type sessionRig struct {
	d    *daemon.Daemon
	c    *rpc.Client
	path string
	host *fakeHost
	wts  *fakeWorktrees
	now  time.Time
}

func startSessions(t *testing.T, store *memStore, setup app.SetupFunc) sessionRig {
	t.Helper()
	r := sessionRig{host: &fakeHost{}, wts: &fakeWorktrees{}, now: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	store.snap.Workspaces = append(store.snap.Workspaces, singleWS, orchWS)
	for _, s := range store.snap.Sessions {
		r.host.panes = append(r.host.panes, app.PaneInfo{ID: app.PaneID(s.Pane), Alive: true})
	}
	r.d, r.path = start(t, store,
		daemon.WithHarnesses(r.host, claude.Adapter{}, codex.Adapter{}),
		daemon.WithSessions(r.wts, setup, "/h/worktrees"),
		daemon.WithClock(func() time.Time { return r.now }))
	r.c = dial(t, r.path)
	return r
}

func (r sessionRig) state(t *testing.T) rpc.State {
	t.Helper()
	sub, err := dial(t, r.path).Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return sub.State
}

func TestNewSessionInASingleRepoStartsInAFreshWorktree(t *testing.T) {
	var setupIn []string
	r := startSessions(t, &memStore{}, func(_ context.Context, dir string) error {
		setupIn = append(setupIn, dir)
		return nil
	})
	var got domain.Session
	params := rpc.NewSessionParams{Workspace: "/src/api", WorkItem: "https://linear.app/acme/issue/ENG-1/fix-login", Harness: "claude", Model: "opus", Effort: "high"}
	if err := r.c.Call(context.Background(), rpc.MethodNewSession, params, &got); err != nil {
		t.Fatal(err)
	}
	if want := []addedWorktree{{"/src/api", "/h/worktrees/api/eng-1", "eng-1", "origin/main"}}; !reflect.DeepEqual(r.wts.added, want) {
		t.Fatalf("added %+v", r.wts.added)
	}
	if !slices.Equal(setupIn, []string{"/h/worktrees/api/eng-1"}) {
		t.Fatalf("setup ran in %v", setupIn)
	}
	wantSpec := app.PaneSpec{Name: "eng-1", Dir: "/h/worktrees/api/eng-1",
		Command: []string{"claude", "--model", "opus", "--effort", "high", "--", "https://linear.app/acme/issue/ENG-1/fix-login"}}
	if len(r.host.specs) != 1 || !reflect.DeepEqual(r.host.specs[0], wantSpec) {
		t.Fatalf("specs %+v", r.host.specs)
	}
	if got.Pane != "%7" || got.State != domain.StateIdle || got.Harness != domain.HarnessClaude || len(got.WorktreeIDs) != 1 || got.TaskID == "" {
		t.Fatalf("session %+v", got)
	}

	st := r.state(t)
	if len(st.Tasks) != 1 || st.Tasks[0].ID != got.TaskID || st.Tasks[0].Source != domain.TaskLinear || st.Tasks[0].Ref != "ENG-1" {
		t.Fatalf("tasks %+v", st.Tasks)
	}
	wantWT := domain.Worktree{ID: got.WorktreeIDs[0], Repo: "api", Path: "/h/worktrees/api/eng-1", Branch: "eng-1"}
	if len(st.Worktrees) != 1 || !reflect.DeepEqual(st.Worktrees[0], wantWT) {
		t.Fatalf("worktrees %+v", st.Worktrees)
	}
	for _, w := range st.Workspaces {
		if w.Root == "/src/api" && !w.LastUsed.Equal(r.now) {
			t.Fatalf("workspace not marked used: %+v", w)
		}
	}
}

func TestNewSessionAtAnOrchestrationRootStartsAtTheRoot(t *testing.T) {
	r := startSessions(t, &memStore{}, nil)
	var got domain.Session
	if err := r.c.Call(context.Background(), rpc.MethodNewSession, rpc.NewSessionParams{Workspace: "/src/shop", WorkItem: "tidy up", Harness: "codex"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(r.wts.added) != 0 || got.WorktreeIDs != nil || got.Harness != domain.HarnessCodex {
		t.Fatalf("added %v, session %+v", r.wts.added, got)
	}
	if len(r.host.specs) != 1 || r.host.specs[0].Dir != "/src/shop" || r.host.specs[0].Command[0] != "codex" {
		t.Fatalf("specs %+v", r.host.specs)
	}
	if st := r.state(t); len(st.Worktrees) != 0 || len(st.Tasks) != 1 || st.Tasks[0].Text != "tidy up" {
		t.Fatalf("state %+v", st)
	}
}

func TestNewSessionWithoutAWorkspaceUsesTheLastUsed(t *testing.T) {
	r := startSessions(t, &memStore{}, nil)
	if err := r.c.Call(context.Background(), rpc.MethodNewSession, rpc.NewSessionParams{WorkItem: "x", Harness: "claude"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(r.host.specs) != 1 || r.host.specs[0].Dir != "/src/shop" {
		t.Fatalf("specs %+v", r.host.specs)
	}
}

func TestNewSessionOnTheSameWorkItemJoinsItsTaskInANewWorktree(t *testing.T) {
	r := startSessions(t, &memStore{}, nil)
	var a, b domain.Session
	p := rpc.NewSessionParams{Workspace: "/src/api", WorkItem: "https://github.com/acme/api/pull/42", Harness: "claude"}
	if err := r.c.Call(context.Background(), rpc.MethodNewSession, p, &a); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Call(context.Background(), rpc.MethodNewSession, p, &b); err != nil {
		t.Fatal(err)
	}
	if a.TaskID != b.TaskID || a.ID == b.ID || a.WorktreeIDs[0] == b.WorktreeIDs[0] {
		t.Fatalf("a %+v, b %+v", a, b)
	}
	if len(r.wts.added) != 2 || r.wts.added[1].path != "/h/worktrees/api/api-42-2" {
		t.Fatalf("added %+v", r.wts.added)
	}
	if st := r.state(t); len(st.Tasks) != 1 || len(st.Sessions) != 2 {
		t.Fatalf("state %+v", st)
	}
}

func TestNewSessionErrors(t *testing.T) {
	r := startSessions(t, &memStore{}, nil)
	var rerr *rpc.Error
	err := r.c.Call(context.Background(), rpc.MethodNewSession, rpc.NewSessionParams{Workspace: "/nope", WorkItem: "x", Harness: "claude"}, nil)
	if !errors.As(err, &rerr) || rerr.Code != rpc.CodeNotFound {
		t.Fatalf("unknown workspace: %v", err)
	}
	err = r.c.Call(context.Background(), rpc.MethodNewSession, rpc.NewSessionParams{Workspace: "/src/api", WorkItem: "x", Harness: "other"}, nil)
	if !errors.As(err, &rerr) || rerr.Code != rpc.CodeBadRequest {
		t.Fatalf("unknown harness: %v", err)
	}
	r.wts.err = errors.New("branch exists")
	err = r.c.Call(context.Background(), rpc.MethodNewSession, rpc.NewSessionParams{Workspace: "/src/api", WorkItem: "x", Harness: "claude"}, nil)
	if !errors.As(err, &rerr) || rerr.Code != rpc.CodeFailed {
		t.Fatalf("worktree failure: %v", err)
	}
	if st := r.state(t); len(st.Sessions) != 0 || len(st.Tasks) != 0 {
		t.Fatalf("a failed start left %+v", st)
	}
}

func TestNewSessionWithNoWorkspaceRegistered(t *testing.T) {
	host := &fakeHost{}
	_, path := start(t, &memStore{}, daemon.WithHarnesses(host, claude.Adapter{}), daemon.WithSessions(&fakeWorktrees{}, nil, "/h"))
	var rerr *rpc.Error
	err := dial(t, path).Call(context.Background(), rpc.MethodNewSession, rpc.NewSessionParams{WorkItem: "x", Harness: "claude"}, nil)
	if !errors.As(err, &rerr) || rerr.Code != rpc.CodeBadRequest {
		t.Fatalf("no workspace: %v", err)
	}
}

func TestEndSessionKillsThePaneAndKeepsTheSessionListed(t *testing.T) {
	r := startSessions(t, &memStore{}, nil)
	var s domain.Session
	if err := r.c.Call(context.Background(), rpc.MethodNewSession, rpc.NewSessionParams{Workspace: "/src/api", WorkItem: "x", Harness: "claude"}, &s); err != nil {
		t.Fatal(err)
	}
	var ended domain.Session
	if err := r.c.Call(context.Background(), rpc.MethodEndSession, rpc.SessionRef{ID: s.ID}, &ended); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.host.killed, []app.PaneID{"%7"}) || ended.State != domain.StateIdle || ended.Pane != "" {
		t.Fatalf("killed %v, session %+v", r.host.killed, ended)
	}
	st := r.state(t)
	if len(st.Sessions) != 1 || st.Sessions[0].Pane != "" || len(st.Worktrees) != 1 {
		t.Fatalf("state %+v", st)
	}
	var rerr *rpc.Error
	if err := r.c.Call(context.Background(), rpc.MethodEndSession, rpc.SessionRef{ID: "nope"}, nil); !errors.As(err, &rerr) || rerr.Code != rpc.CodeNotFound {
		t.Fatalf("unknown session: %v", err)
	}
}

func TestFocusSessionShowsItsPaneInTheMainSlot(t *testing.T) {
	store := &memStore{}
	store.snap.Sessions = []domain.Session{
		{ID: "a", Pane: "%7", State: domain.StateDone, Unread: true},
		{ID: "b", Pane: "%8", State: domain.StateRunning, Focused: true},
	}
	r := startSessions(t, store, nil)
	focused := func() map[string]bool {
		out := map[string]bool{}
		for _, s := range r.state(t).Sessions {
			out[s.ID] = s.Focused
		}
		return out
	}
	if err := r.c.Call(context.Background(), rpc.MethodSessionFocus, rpc.SessionFocusParams{ID: "a"}, nil); err != nil {
		t.Fatalf("focus without a client layout: %v", err)
	}
	if len(r.host.shown) != 0 || !reflect.DeepEqual(focused(), map[string]bool{"a": true, "b": false}) {
		t.Fatalf("without a layout: shown %+v, focused %v", r.host.shown, focused())
	}
	clientHost := &fakeClientHost{}
	r.d.SetClientHost(clientHost)
	opened, err := r.c.OpenClient(context.Background(), rpc.OpenClientParams{Command: []string{"agentws", "tui"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.c.Call(context.Background(), rpc.MethodSessionFocus, rpc.SessionFocusParams{ID: "b"}, nil); err != nil {
		t.Fatal(err)
	}
	slot := app.Slot(opened.Slot)
	if !reflect.DeepEqual(r.host.shown, []shown{{"%8", slot}}) || !slices.Equal(clientHost.focused, []app.Slot{slot}) {
		t.Fatalf("shown %+v, focused %v", r.host.shown, clientHost.focused)
	}
	if !reflect.DeepEqual(focused(), map[string]bool{"a": false, "b": true}) {
		t.Fatalf("focused %v", focused())
	}
}

func TestRestoredSessionsWhosePaneIsGoneAreEnded(t *testing.T) {
	store := &memStore{}
	store.snap.Sessions = []domain.Session{
		{ID: "alive", Pane: "%1", State: domain.StateRunning},
		{ID: "gone", Pane: "%2", State: domain.StateWaiting},
	}
	host := &fakeHost{panes: []app.PaneInfo{{ID: "%1", Alive: true}}}
	_, path := start(t, store, daemon.WithHarnesses(host, claude.Adapter{}))
	c := dial(t, path)
	waitFor(t, func() bool {
		sub, err := c.Subscribe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]domain.Session{}
		for _, s := range sub.State.Sessions {
			byID[s.ID] = s
		}
		return byID["gone"].State == domain.StateIdle && byID["gone"].Pane == "" &&
			byID["alive"].State == domain.StateRunning && byID["alive"].Pane == "%1"
	})
}

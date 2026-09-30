package app_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

func newSession(plan domain.SessionPlan) app.NewSession {
	return app.NewSession{
		ID: "s1", Task: domain.Task{ID: "t1"}, Plan: plan, Harness: fakeHarness{},
		Name: "eng-1", Model: "m", Effort: "high", Prompt: "do it",
	}
}

var singlePlan = domain.SessionPlan{Dir: "/h/api/eng-1", Worktree: &domain.WorktreePlan{
	Repo: "api", RepoPath: "/src/api", Path: "/h/api/eng-1", Branch: "eng-1", Base: "origin/main",
}}

func TestStartSessionInASingleRepoAddsTheWorktreeRunsSetupThenLaunchesWhereGitPutIt(t *testing.T) {
	var log []string
	host := &fakeHost{log: &log}
	wts := &fakeWorktrees{log: &log}
	s := app.Sessions{Host: host, Worktrees: wts, Setup: func(_ context.Context, dir string) error {
		log = append(log, "setup "+dir)
		return nil
	}}
	got, err := s.Start(context.Background(), newSession(singlePlan))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"add /h/api/eng-1", "setup /real/h/api/eng-1", "create /real/h/api/eng-1"}; !slices.Equal(log, want) {
		t.Fatalf("steps %v, want %v", log, want)
	}
	if want := []addedWorktree{{"/src/api", "/h/api/eng-1", "eng-1", "origin/main"}}; !reflect.DeepEqual(wts.added, want) {
		t.Fatalf("added %+v", wts.added)
	}
	wantSpec := app.PaneSpec{Name: "eng-1", Dir: "/real/h/api/eng-1", Command: []string{"agent", "m", "high", "do it"}}
	if len(host.created) != 1 || !reflect.DeepEqual(host.created[0], wantSpec) {
		t.Fatalf("created %+v", host.created)
	}
	wantSession := domain.Session{
		ID: "s1", TaskID: "t1", Harness: domain.HarnessCodex, Pane: "%9", Model: "m", Effort: "high",
		State: domain.StateIdle, WorktreeIDs: []string{"/real/h/api/eng-1"},
	}
	if !reflect.DeepEqual(got.Session, wantSession) {
		t.Fatalf("session %+v", got.Session)
	}
	wantWorktree := &domain.Worktree{ID: "/real/h/api/eng-1", Repo: "/real/src/api", Path: "/real/h/api/eng-1", Branch: "eng-1", SessionID: "s1"}
	if !reflect.DeepEqual(got.Worktree, wantWorktree) {
		t.Fatalf("worktree %+v", got.Worktree)
	}
}

func TestStartSessionAtAnOrchestrationRootCreatesNoWorktree(t *testing.T) {
	host := &fakeHost{}
	wts := &fakeWorktrees{}
	setups := 0
	s := app.Sessions{Host: host, Worktrees: wts, Setup: func(context.Context, string) error { setups++; return nil }}
	got, err := s.Start(context.Background(), newSession(domain.SessionPlan{Dir: "/src/shop"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(wts.added) != 0 || setups != 0 || got.Worktree != nil || got.Session.WorktreeIDs != nil {
		t.Fatalf("added %v, setups %d, started %+v", wts.added, setups, got)
	}
	if len(host.created) != 1 || host.created[0].Dir != "/src/shop" || got.Session.Pane != "%9" {
		t.Fatalf("created %+v, session %+v", host.created, got.Session)
	}
}

func TestStartSessionWithoutSetupStillLaunches(t *testing.T) {
	host := &fakeHost{}
	s := app.Sessions{Host: host, Worktrees: &fakeWorktrees{}}
	if _, err := s.Start(context.Background(), newSession(singlePlan)); err != nil || len(host.created) != 1 {
		t.Fatalf("err %v, created %d", err, len(host.created))
	}
}

func TestStartSessionStopsAtTheFirstFailure(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		wts     *fakeWorktrees
		setup   app.SetupFunc
		hostErr error
		created int
	}{
		{"worktree add", &fakeWorktrees{err: boom}, nil, nil, 0},
		{"setup", &fakeWorktrees{}, func(context.Context, string) error { return boom }, nil, 0},
		{"pane", &fakeWorktrees{}, nil, boom, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host := &fakeHost{createErr: c.hostErr}
			s := app.Sessions{Host: host, Worktrees: c.wts, Setup: c.setup}
			got, err := s.Start(context.Background(), newSession(singlePlan))
			if !errors.Is(err, boom) || got.Session.ID != "" || got.Worktree != nil {
				t.Fatalf("started %+v, err %v", got, err)
			}
			if len(host.created) != c.created {
				t.Fatalf("a pane was created after the %s failed", c.name)
			}
		})
	}
}

func TestEndSessionKillsThePaneAndIdlesTheSession(t *testing.T) {
	host := &fakeHost{}
	s := app.Sessions{Host: host}
	running := domain.Session{ID: "a", Pane: "%3", State: domain.StateRunning, WorktreeIDs: []string{"w"}}
	got, err := s.End(context.Background(), running)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(host.killed, []app.PaneID{"%3"}) || !reflect.DeepEqual(got, running.End()) {
		t.Fatalf("killed %v, session %+v", host.killed, got)
	}
	if _, err := s.End(context.Background(), got); err != nil || len(host.killed) != 1 {
		t.Fatalf("ending an ended session: killed %v, err %v", host.killed, err)
	}
}

func TestEndSessionKeepsTheSessionWhenThePaneWillNotDie(t *testing.T) {
	boom := errors.New("boom")
	running := domain.Session{ID: "a", Pane: "%3", State: domain.StateRunning}
	got, err := app.Sessions{Host: &fakeHost{killErr: boom}}.End(context.Background(), running)
	if !errors.Is(err, boom) || !reflect.DeepEqual(got, running) {
		t.Fatalf("session %+v, err %v", got, err)
	}
}

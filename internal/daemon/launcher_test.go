package daemon_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/adapters/claude"
	"github.com/giovaniif/agent-workspace/internal/adapters/codex"
	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/daemon"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

type launcherRig struct {
	c    *rpc.Client
	path string
	host *fakeHost
}

func startLauncher(t *testing.T, maxParallel int, titles app.TitleResolver) launcherRig {
	t.Helper()
	store := &memStore{}
	store.snap.Workspaces = append(store.snap.Workspaces, orchWS)
	host := &fakeHost{distinct: true}
	opts := []daemon.Option{
		daemon.WithHarnesses(host, claude.Adapter{}, codex.Adapter{}),
		daemon.WithSessions(&fakeWorktrees{}, nil, "/h/worktrees"),
		daemon.WithLauncher(maxParallel),
	}
	if titles != nil {
		opts = append(opts, daemon.WithTitles(titles))
	}
	_, path := start(t, store, opts...)
	return launcherRig{c: dial(t, path), path: path, host: host}
}

func issueURLs(n int) string {
	var lines []string
	for i := 1; i <= n; i++ {
		lines = append(lines, fmt.Sprintf("https://linear.app/acme/issue/ENG-%d/issue-number-%d", i, i))
	}
	return strings.Join(lines, "\n")
}

func (r launcherRig) enqueue(t *testing.T, input string) rpc.LauncherEnqueued {
	t.Helper()
	var got rpc.LauncherEnqueued
	p := rpc.LauncherEnqueueParams{Workspace: "/src/shop", Input: input, Harness: "claude", Model: "opus", Effort: "high"}
	if err := r.c.Call(context.Background(), rpc.MethodLauncherEnqueue, p, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func (r launcherRig) state(t *testing.T) rpc.State {
	t.Helper()
	sub, err := dial(t, r.path).Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return sub.State
}

func (r launcherRig) started() int {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	return len(r.host.specs)
}

func (r launcherRig) waitStarted(t *testing.T, n int) {
	t.Helper()
	waitFor(t, func() bool { return r.started() >= n })
	time.Sleep(100 * time.Millisecond)
	if got := r.started(); got != n {
		t.Fatalf("%d sessions started, want %d", got, n)
	}
}

func (r launcherRig) waitQueue(t *testing.T, refs ...string) {
	t.Helper()
	var got []string
	waitFor(t, func() bool {
		got = nil
		for _, i := range r.state(t).Queue {
			got = append(got, i.Ref)
		}
		return strings.Join(got, " ") == strings.Join(refs, " ")
	})
}

func (r launcherRig) finish(t *testing.T, pane string) {
	t.Helper()
	if err := r.c.Call(context.Background(), rpc.MethodHook, rpc.Hook{Harness: "claude", Event: "Stop", Pane: pane}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLauncherStartsMaxParallelSessionsAndQueuesTheRest(t *testing.T) {
	r := startLauncher(t, 3, nil)
	got := r.enqueue(t, issueURLs(5))
	if len(got.Queued) != 5 || len(got.Rejected) != 0 {
		t.Fatalf("enqueued %+v", got)
	}
	r.waitStarted(t, 3)
	r.waitQueue(t, "ENG-4", "ENG-5")
	for _, spec := range r.host.specs {
		if spec.Command[0] != "claude" || spec.Dir != "/src/shop" {
			t.Fatalf("spec %+v", spec)
		}
	}
	if st := r.state(t); len(st.Sessions) != 3 || len(st.Tasks) != 3 {
		t.Fatalf("%d sessions and %d tasks, want 3 and 3", len(st.Sessions), len(st.Tasks))
	}
}

func TestLauncherQueueDrainsAsSessionsFinish(t *testing.T) {
	r := startLauncher(t, 3, nil)
	r.enqueue(t, issueURLs(5))
	r.waitStarted(t, 3)

	r.finish(t, "%7")
	r.waitStarted(t, 4)
	r.waitQueue(t, "ENG-5")

	r.finish(t, "%8")
	r.waitStarted(t, 5)
	r.waitQueue(t)
	r.finish(t, "%9")
	time.Sleep(100 * time.Millisecond)
	if got := r.started(); got != 5 {
		t.Fatalf("%d sessions started after the queue emptied, want 5", got)
	}
}

func TestLauncherFreesASlotWhenASessionIsEnded(t *testing.T) {
	r := startLauncher(t, 1, nil)
	r.enqueue(t, issueURLs(2))
	r.waitStarted(t, 1)
	r.waitQueue(t, "ENG-2")
	sessions := r.state(t).Sessions
	if err := r.c.Call(context.Background(), rpc.MethodEndSession, rpc.SessionRef{ID: sessions[0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	r.waitStarted(t, 2)
}

func TestLauncherDoesNotCountSessionsItDidNotStart(t *testing.T) {
	r := startLauncher(t, 1, nil)
	var own domain.Session
	p := rpc.NewSessionParams{Workspace: "/src/shop", WorkItem: "tidy up", Harness: "claude"}
	if err := r.c.Call(context.Background(), rpc.MethodNewSession, p, &own); err != nil {
		t.Fatal(err)
	}
	r.enqueue(t, issueURLs(1))
	r.waitStarted(t, 2)
}

type refTitles map[string]string

func (m refTitles) Title(_ context.Context, task domain.Task) (string, error) {
	return m[task.Ref], nil
}

func TestLauncherNamesEachSessionAfterItsIssue(t *testing.T) {
	r := startLauncher(t, 3, refTitles{"ENG-1": "Add search", "ENG-2": "Fix login redirect"})
	r.enqueue(t, issueURLs(2))
	r.waitStarted(t, 2)
	want := map[string]string{"ENG-1": "Add search", "ENG-2": "Fix login redirect"}
	waitFor(t, func() bool {
		st := r.state(t)
		named := 0
		for _, x := range st.Sessions {
			for _, task := range st.Tasks {
				if task.ID == x.TaskID && domain.NameFor(task, nil) == want[task.Ref] {
					named++
				}
			}
		}
		return named == 2
	})
}

func TestLauncherKeepsAFailedStartInTheQueueAndMovesOn(t *testing.T) {
	r := startLauncher(t, 1, nil)
	r.host.mu.Lock()
	r.host.failFirst = true
	r.host.mu.Unlock()
	r.enqueue(t, issueURLs(2))
	r.waitStarted(t, 1)
	waitFor(t, func() bool {
		q := r.state(t).Queue
		return len(q) == 1 && q[0].Ref == "ENG-1" && q[0].Err != "" && !q[0].Starting
	})
}

func TestLauncherRetargetsAQueuedIssueToCodex(t *testing.T) {
	r := startLauncher(t, 1, nil)
	r.enqueue(t, issueURLs(2))
	r.waitStarted(t, 1)
	r.waitQueue(t, "ENG-2")
	queued := r.state(t).Queue[0]
	p := rpc.LauncherRetargetParams{ID: queued.ID, Harness: "codex", Model: "gpt-5", Effort: "high"}
	if err := r.c.Call(context.Background(), rpc.MethodLauncherRetarget, p, nil); err != nil {
		t.Fatal(err)
	}
	r.finish(t, "%7")
	r.waitStarted(t, 2)
	if spec := r.host.specs[1]; spec.Command[0] != "codex" {
		t.Fatalf("second session started as %v, want codex", spec.Command)
	}
	if err := r.c.Call(context.Background(), rpc.MethodLauncherRetarget, p, nil); err == nil {
		t.Fatal("retargeted an issue that already started")
	}
}

func TestLauncherDropTakesAQueuedIssueOffTheQueue(t *testing.T) {
	r := startLauncher(t, 1, nil)
	r.enqueue(t, issueURLs(3))
	r.waitStarted(t, 1)
	r.waitQueue(t, "ENG-2", "ENG-3")
	dropped := r.state(t).Queue[0]
	if err := r.c.Call(context.Background(), rpc.MethodLauncherDrop, rpc.LauncherItemRef{ID: dropped.ID}, nil); err != nil {
		t.Fatal(err)
	}
	r.waitQueue(t, "ENG-3")
	r.finish(t, "%7")
	r.waitStarted(t, 2)
	if got := r.host.specs[1].Command; got[len(got)-1] != "https://linear.app/acme/issue/ENG-3/issue-number-3" {
		t.Fatalf("second session started with %v, want ENG-3", got)
	}
	if err := r.c.Call(context.Background(), rpc.MethodLauncherDrop, rpc.LauncherItemRef{ID: "nope"}, nil); err == nil {
		t.Fatal("dropped an issue that is not queued")
	}
}

func TestLauncherQueuesAnIssueOnlyOnce(t *testing.T) {
	r := startLauncher(t, 1, nil)
	r.enqueue(t, issueURLs(2))
	r.waitStarted(t, 1)
	got := r.enqueue(t, issueURLs(2))
	if len(got.Queued) != 0 {
		t.Fatalf("queued again: %+v", got)
	}
	r.waitQueue(t, "ENG-2")
}

func TestLauncherRefusesInputWithoutALinearIssue(t *testing.T) {
	r := startLauncher(t, 3, nil)
	err := r.c.Call(context.Background(), rpc.MethodLauncherEnqueue, rpc.LauncherEnqueueParams{Input: "fix the login page https://github.com/acme/api/pull/3", Harness: "claude"}, nil)
	rerr, ok := err.(*rpc.Error)
	if !ok || rerr.Code != rpc.CodeBadRequest {
		t.Fatalf("error %v, want bad_request", err)
	}
	if r.started() != 0 {
		t.Fatal("started a session")
	}
}

func TestLauncherReportsWhatItDidNotTakeForAnIssue(t *testing.T) {
	r := startLauncher(t, 3, nil)
	got := r.enqueue(t, "https://linear.app/acme/issue/ENG-1/a and https://github.com/acme/api/pull/3")
	if len(got.Queued) != 1 || len(got.Rejected) != 2 {
		t.Fatalf("enqueued %+v", got)
	}
}

func TestLauncherQueueReachesSubscribersAsDiffs(t *testing.T) {
	r := startLauncher(t, 1, nil)
	sub, err := dial(t, r.path).Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r.enqueue(t, issueURLs(2))
	deadline := time.After(2 * time.Second)
	for {
		select {
		case d := <-sub.Diffs:
			if d.Queue != nil && len(*d.Queue) == 1 && (*d.Queue)[0].Ref == "ENG-2" && !(*d.Queue)[0].Starting {
				return
			}
		case <-deadline:
			t.Fatal("no queue diff with ENG-2 waiting")
		}
	}
}

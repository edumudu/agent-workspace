package daemon_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/adapters/sqlite"
	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/daemon"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

type memStore struct {
	mu   sync.Mutex
	snap app.Snapshot
}

func (s *memStore) PutWorkspace(w domain.Workspace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Workspaces = append(s.snap.Workspaces, w)
}

func (s *memStore) PutTask(x domain.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Tasks = append(s.snap.Tasks, x)
}

func (s *memStore) PutWorktree(w domain.Worktree) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Worktrees = append(s.snap.Worktrees, w)
}

func (s *memStore) PutSession(x domain.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Sessions = append(s.snap.Sessions, x)
}

func (s *memStore) Load() (app.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap, nil
}

func (s *memStore) Flush() error { return nil }
func (s *memStore) Close() error { return nil }

func shortDir(t *testing.T) string {
	t.Helper()
	// why: macOS caps Unix socket paths at 104 bytes, and t.TempDir() can exceed it.
	dir, err := os.MkdirTemp("/tmp", "agentws-d")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func start(t *testing.T, store app.Store) (*daemon.Daemon, string) {
	t.Helper()
	d, err := daemon.New(store, 1234)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(shortDir(t), "agentws.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = d.Serve(ctx, ln)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return d, path
}

func dial(t *testing.T, path string) *rpc.Client {
	t.Helper()
	c, err := rpc.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func next(t *testing.T, diffs <-chan rpc.Diff) rpc.Diff {
	t.Helper()
	select {
	case d, ok := <-diffs:
		if !ok {
			t.Fatal("diff stream closed")
		}
		return d
	case <-time.After(2 * time.Second):
		t.Fatal("no diff")
	}
	return rpc.Diff{}
}

func TestSubscribersAllGetEveryDiffInTheSameOrder(t *testing.T) {
	d, path := start(t, &memStore{})
	ctx := context.Background()
	a, err := dial(t, path).Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := dial(t, path).Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const n = 50
	for i := range n {
		if i%2 == 0 {
			d.Post(daemon.SessionChanged{Session: domain.Session{ID: fmt.Sprint("s", i)}})
		} else {
			d.Post(daemon.WorktreeChanged{Worktree: domain.Worktree{ID: fmt.Sprint("w", i)}})
		}
	}
	for i := range n {
		da, db := next(t, a.Diffs), next(t, b.Diffs)
		ja, _ := json.Marshal(da)
		jb, _ := json.Marshal(db)
		if string(ja) != string(jb) {
			t.Fatalf("diff %d differs: %+v vs %+v", i, da, db)
		}
		if da.Seq != a.State.Seq+uint64(i)+1 {
			t.Fatalf("diff %d has seq %d after state seq %d", i, da.Seq, a.State.Seq)
		}
		if (i%2 == 0) != (da.Session != nil) {
			t.Fatalf("diff %d out of order: %+v", i, da)
		}
	}
}

func TestSubscribeStartsFromTheCurrentState(t *testing.T) {
	d, path := start(t, &memStore{snap: app.Snapshot{Sessions: []domain.Session{{ID: "old"}}}})
	d.Post(daemon.SessionChanged{Session: domain.Session{ID: "new"}})
	ctx := context.Background()
	c := dial(t, path)
	deadline := time.Now().Add(2 * time.Second)
	for {
		sub, err := c.Subscribe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(sub.State.Sessions) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("state %+v", sub.State)
		}
	}
}

func TestStateChangesUpdateTheSameSession(t *testing.T) {
	d, path := start(t, &memStore{})
	d.Post(daemon.SessionChanged{Session: domain.Session{ID: "a", State: domain.StateRunning}})
	d.Post(daemon.SessionChanged{Session: domain.Session{ID: "a", State: domain.StateDone}})
	d.Post(daemon.WorktreeChanged{Worktree: domain.Worktree{ID: "w"}})
	st := waitStatus(t, dial(t, path), func(s rpc.Status) bool { return s.Worktrees == 1 })
	if st.Sessions != 1 {
		t.Fatalf("status %+v", st)
	}
}

func TestChangesArePersisted(t *testing.T) {
	store := &memStore{}
	d, path := start(t, store)
	d.Post(daemon.SessionChanged{Session: domain.Session{ID: "a"}})
	waitStatus(t, dial(t, path), func(s rpc.Status) bool { return s.Sessions == 1 })
	snap, _ := store.Load()
	if len(snap.Sessions) != 1 || snap.Sessions[0].ID != "a" {
		t.Fatalf("stored %+v", snap)
	}
}

func waitStatus(t *testing.T, c *rpc.Client, ok func(rpc.Status) bool) rpc.Status {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		st, err := c.Status(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if ok(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("status %+v", st)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStatusReportsPIDAndRestoredCounts(t *testing.T) {
	store := &memStore{snap: app.Snapshot{
		Sessions:  []domain.Session{{ID: "a"}, {ID: "b"}},
		Worktrees: []domain.Worktree{{ID: "w"}},
	}}
	_, path := start(t, store)
	st, err := dial(t, path).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.PID != 1234 || st.Sessions != 2 || st.Worktrees != 1 || st.StartedAt.IsZero() {
		t.Fatalf("status %+v", st)
	}
}

func rawCall(t *testing.T, path, line string) rpc.Response {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	got, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var resp rpc.Response
	if err := json.Unmarshal(got, &resp); err != nil {
		t.Fatalf("%s: %v", got, err)
	}
	return resp
}

func TestBadRequestsGetStructuredErrors(t *testing.T) {
	_, path := start(t, &memStore{})
	tests := []struct {
		line string
		id   uint64
		code string
	}{
		{`{"v":2,"id":7,"method":"status"}`, 7, rpc.CodeUnsupportedVersion},
		{`{"id":8,"method":"status"}`, 8, rpc.CodeUnsupportedVersion},
		{`{"v":1,"id":9,"method":"nope"}`, 9, rpc.CodeUnknownMethod},
		{`not json`, 0, rpc.CodeBadRequest},
	}
	for _, tt := range tests {
		resp := rawCall(t, path, tt.line)
		if resp.V != rpc.Version || resp.ID != tt.id || resp.Error == nil || resp.Error.Code != tt.code || resp.Error.Message == "" {
			t.Errorf("%s: got %+v %+v", tt.line, resp, resp.Error)
		}
	}
}

func TestClientSurfacesServerErrors(t *testing.T) {
	_, path := start(t, &memStore{})
	c := dial(t, path)
	var rerr *rpc.Error
	if err := c.Call(context.Background(), "nope", nil, nil); !errors.As(err, &rerr) || rerr.Code != rpc.CodeUnknownMethod {
		t.Fatalf("err %v", err)
	}
}

func TestASlowSubscriberDoesNotStallTheLoop(t *testing.T) {
	d, path := start(t, &memStore{})
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte(`{"v":1,"id":1,"method":"subscribe"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	blob := strings.Repeat("x", 4096)
	for i := range 5000 {
		d.Post(daemon.SessionChanged{Session: domain.Session{ID: fmt.Sprint(i), Model: blob}})
	}
	waitStatus(t, dial(t, path), func(s rpc.Status) bool { return s.Sessions == 5000 })
}

func TestHookEventUpdatesTheSessionOnItsPane(t *testing.T) {
	d, path := start(t, &memStore{})
	c := dial(t, path)
	ctx := context.Background()
	sub, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d.Post(daemon.SessionChanged{Session: domain.Session{ID: "a", Pane: "%3", State: domain.StateRunning}})
	next(t, sub.Diffs)
	hook := rpc.Hook{Harness: "claude", Event: "Stop", Pane: "%3", At: time.Now(), Payload: json.RawMessage(`{}`)}
	if err := c.Call(ctx, rpc.MethodHook, hook, nil); err != nil {
		t.Fatal(err)
	}
	diff := next(t, sub.Diffs)
	if diff.Session == nil || diff.Session.ID != "a" || diff.Session.State != domain.StateDone {
		t.Fatalf("diff %+v", diff)
	}
}

func TestHookFromAnUnknownPaneOrEventIsIgnored(t *testing.T) {
	d, path := start(t, &memStore{})
	c := dial(t, path)
	ctx := context.Background()
	sub, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d.Post(daemon.SessionChanged{Session: domain.Session{ID: "a", Pane: "%3", State: domain.StateRunning}})
	next(t, sub.Diffs)
	for _, h := range []rpc.Hook{
		{Harness: "claude", Event: "Stop", Pane: "%9"},
		{Harness: "claude", Event: "Bogus", Pane: "%3"},
	} {
		if err := c.Call(ctx, rpc.MethodHook, h, nil); err != nil {
			t.Fatal(err)
		}
	}
	d.Post(daemon.WorktreeChanged{Worktree: domain.Worktree{ID: "w"}})
	if diff := next(t, sub.Diffs); diff.Worktree == nil {
		t.Fatalf("hook produced %+v", diff)
	}
}

func TestOnlyOneDaemonHoldsTheLock(t *testing.T) {
	home := shortDir(t)
	first, err := daemon.Acquire(home)
	if err != nil {
		t.Fatal(err)
	}
	_, err = daemon.Acquire(home)
	if !errors.Is(err, daemon.ErrAlreadyRunning) || !strings.Contains(err.Error(), fmt.Sprint(os.Getpid())) {
		t.Fatalf("second acquire: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	again, err := daemon.Acquire(home)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	_ = again.Release()
}

func TestRunServesStateRestoredFromTheStore(t *testing.T) {
	home := shortDir(t)
	store, err := sqlite.Open(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	store.PutSession(domain.Session{ID: "a"})
	store.PutWorktree(domain.Worktree{ID: "w"})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- daemon.Run(ctx, home) }()
		c, err := rpc.Connect(ctx, rpc.SocketPath(home), func() error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		st, err := c.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if st.Sessions != 1 || st.Worktrees != 1 {
			t.Fatalf("restored %+v", st)
		}
		_ = c.Close()
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

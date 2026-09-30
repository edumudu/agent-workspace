package sqlite

import (
	"bufio"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func reopen(t *testing.T, s *Store, path string) *Store {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestRoundTripsEveryDomainType(t *testing.T) {
	s, path := openTemp(t)
	ws := domain.Workspace{Root: "/src/shop", Kind: domain.WorkspaceOrchestration, Repos: []domain.Repo{{Name: "api", Path: "/src/shop/api"}, {Name: "web", Path: "/src/shop/web"}}}
	task := domain.Task{ID: "t1", Source: domain.TaskLinear, Ref: "#42", Text: "fix login", IssueTitle: "Login fails", PinnedName: "login"}
	wt := domain.Worktree{ID: "w1", Repo: "api", Path: "/wt/api-42", Branch: "42-login", PR: &domain.PullRequest{Number: 7, Title: "fix login", URL: "https://example.com/pr/7"}, SubtaskSlug: "api"}
	wtNoPR := domain.Worktree{ID: "w2", Repo: "web", Path: "/wt/web-42", Branch: "42-web"}
	sess := domain.Session{ID: "s1", TaskID: "t1", Harness: domain.HarnessCodex, Model: "m", Effort: "high", State: domain.StatePermission, Unread: true, Focused: true, WorktreeIDs: []string{"w1", "w2"}, Usage: domain.Usage{ContextLeftPercent: 40, LimitUsedPercent: 12}}

	s.PutWorkspace(ws)
	s.PutTask(task)
	s.PutWorktree(wt)
	s.PutWorktree(wtNoPR)
	s.PutSession(sess)

	snap, err := reopen(t, s, path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snap.Workspaces, []domain.Workspace{ws}) {
		t.Errorf("workspaces = %+v", snap.Workspaces)
	}
	if !reflect.DeepEqual(snap.Tasks, []domain.Task{task}) {
		t.Errorf("tasks = %+v", snap.Tasks)
	}
	if !reflect.DeepEqual(snap.Worktrees, []domain.Worktree{wt, wtNoPR}) {
		t.Errorf("worktrees = %+v", snap.Worktrees)
	}
	if !reflect.DeepEqual(snap.Sessions, []domain.Session{sess}) {
		t.Errorf("sessions = %+v", snap.Sessions)
	}
}

func TestLaterPutReplacesEarlier(t *testing.T) {
	s, _ := openTemp(t)
	s.PutSession(domain.Session{ID: "s1", State: domain.StateRunning})
	s.PutSession(domain.Session{ID: "s1", State: domain.StateDone})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	snap, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 1 || snap.Sessions[0].State != domain.StateDone {
		t.Errorf("sessions = %+v", snap.Sessions)
	}
}

func TestDeleteWorkspaceRemovesTheRowAndBeatsAnUnflushedPut(t *testing.T) {
	s, path := openTemp(t)
	s.PutWorkspace(domain.Workspace{Root: "/a"})
	s.PutWorkspace(domain.Workspace{Root: "/b"})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s.DeleteWorkspace("/a")
	s.PutWorkspace(domain.Workspace{Root: "/c"})
	s.DeleteWorkspace("/c")
	s.DeleteWorkspace("/b")
	s.PutWorkspace(domain.Workspace{Root: "/b", Kind: domain.WorkspaceSingle})

	snap, err := reopen(t, s, path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Workspaces) != 1 || snap.Workspaces[0].Root != "/b" || snap.Workspaces[0].Kind != domain.WorkspaceSingle {
		t.Errorf("workspaces = %+v, want only /b", snap.Workspaces)
	}
}

func TestWritesReachDiskWithoutExplicitFlush(t *testing.T) {
	s, path := openTemp(t)
	s.PutTask(domain.Task{ID: "t1"})
	time.Sleep(3 * FlushInterval)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("tasks on disk = %d, want 1", n)
	}
}

func TestOpenUpgradesOlderVersion(t *testing.T) {
	src, err := os.ReadFile("testdata/v1.db")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.db")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if v, err := s.Version(); err != nil || v != LatestVersion() || v < 2 {
		t.Fatalf("version = %d, %v; want %d", v, err, LatestVersion())
	}
	snap, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 1 || snap.Sessions[0].ID != "s-old" {
		t.Errorf("sessions from v1 = %+v", snap.Sessions)
	}
	s.PutWorktree(domain.Worktree{ID: "w1"})
	if err := s.Flush(); err != nil {
		t.Fatalf("v2 table not usable after upgrade: %v", err)
	}
}

func TestReopeningLatestKeepsVersion(t *testing.T) {
	s, path := openTemp(t)
	r := reopen(t, s, path)
	if v, err := r.Version(); err != nil || v != LatestVersion() {
		t.Fatalf("version = %d, %v", v, err)
	}
}

func TestDefaultPathHonoursAgentwsHome(t *testing.T) {
	t.Setenv("AGENTWS_HOME", "/tmp/aw")
	if p, _ := DefaultPath(); p != "/tmp/aw/state.db" {
		t.Errorf("path = %q", p)
	}
	t.Setenv("AGENTWS_HOME", "")
	t.Setenv("HOME", "/home/u")
	if p, _ := DefaultPath(); p != "/home/u/.agentws/state.db" {
		t.Errorf("path = %q", p)
	}
}

const writeLoopEnv = "AGENTWS_SQLITE_WRITE_LOOP"

func TestMain(m *testing.M) {
	if path := os.Getenv(writeLoopEnv); path != "" {
		runWriteLoop(path)
		return
	}
	os.Exit(m.Run())
}

func runWriteLoop(path string) {
	s, err := Open(path)
	if err != nil {
		os.Exit(3)
	}
	for i := 1; ; i++ {
		s.PutSession(domain.Session{ID: "s1", Model: strconv.Itoa(i)})
		_, _ = os.Stdout.WriteString(strconv.Itoa(i) + "\n")
		time.Sleep(time.Millisecond)
	}
}

func TestKilledProcessLosesAtMostLast100ms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), writeLoopEnv+"="+path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	type mark struct {
		seq int
		at  time.Time
	}
	marks := make(chan mark, 1<<16)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			n, _ := strconv.Atoi(sc.Text())
			marks <- mark{n, time.Now()}
		}
		close(marks)
	}()

	time.Sleep(700 * time.Millisecond)
	killedAt := time.Now()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	mustHave := 0
	for m := range marks {
		if m.at.Before(killedAt.Add(-100 * time.Millisecond)) {
			mustHave = m.seq
		}
	}
	_ = cmd.Wait()
	if mustHave == 0 {
		t.Fatal("write loop produced nothing")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var check string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil || check != "ok" {
		t.Fatalf("integrity_check = %q, %v", check, err)
	}
	_ = db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	snap, err := s.Load()
	if err != nil || len(snap.Sessions) != 1 {
		t.Fatalf("sessions = %+v, %v", snap.Sessions, err)
	}
	got, _ := strconv.Atoi(snap.Sessions[0].Model)
	if got < mustHave {
		t.Errorf("persisted seq %d, want >= %d (written more than 100 ms before kill)", got, mustHave)
	}
}

func BenchmarkFlush1000SessionUpdates(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "state.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for b.Loop() {
		start := time.Now()
		for i := range 1000 {
			s.PutSession(domain.Session{ID: "s" + strconv.Itoa(i), State: domain.StateRunning, Model: strconv.Itoa(i)})
		}
		if err := s.Flush(); err != nil {
			b.Fatal(err)
		}
		if d := time.Since(start); d > 200*time.Millisecond {
			b.Fatalf("1000 updates flushed in %v, budget 200ms", d)
		}
	}
}

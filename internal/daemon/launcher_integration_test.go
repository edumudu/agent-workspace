//go:build integration

package daemon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/adapters/linear"
	"github.com/giovaniif/agent-workspace/internal/adapters/tmux"
	"github.com/giovaniif/agent-workspace/internal/daemon"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

func fakeLinear(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables struct {
				ID string `json:"id"`
			} `json:"variables"`
		}
		if r.Header.Get("Authorization") != "test-token" || json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad request", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":{"issue":{"title":"Title of %s"}}}`, req.Variables.ID)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLauncherIntegrationFiveIssuesWithMaxParallelThree(t *testing.T) {
	for _, bin := range []string{"git", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip(bin + " not installed")
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	for _, kv := range [][2]string{{"GIT_AUTHOR_NAME", "t"}, {"GIT_AUTHOR_EMAIL", "t@example.com"}, {"GIT_COMMITTER_NAME", "t"}, {"GIT_COMMITTER_EMAIL", "t@example.com"}} {
		t.Setenv(kv[0], kv[1])
	}
	home, err := filepath.EvalSymlinks(shortDir(t))
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(home, "src")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(scratch, "api")
	cloneRepo(t, scratch, repo)
	fakeAgent := filepath.Join(home, "fake-agent")
	if err := os.WriteFile(fakeAgent, []byte("#!/bin/sh\nexec cat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("agentws-it-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		_ = tmux.New(tmux.Config{Socket: socket, ConfigPath: filepath.Join(home, "tmux.conf")}).Close(context.Background())
	})

	api := fakeLinear(t)
	live := serveLive(t, home, socket, fakeAgent,
		daemon.WithLauncher(3),
		daemon.WithTitles(linear.Client{Token: "test-token", Endpoint: api.URL}))
	ctx := context.Background()
	if err := live.c.Call(ctx, rpc.MethodWorkspaceAdd, rpc.WorkspaceAddParams{Path: repo}, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		var list rpc.WorkspaceList
		_ = live.c.Call(ctx, rpc.MethodWorkspaceList, nil, &list)
		for _, w := range list.Workspaces {
			if w.Root == repo && len(w.Repos) == 1 && w.Repos[0].DefaultBranch == "main" {
				return true
			}
		}
		return false
	})

	input := ""
	for i := 1; i <= 5; i++ {
		input += fmt.Sprintf("https://linear.app/acme/issue/ENG-%d/issue-%d\n", i, i)
	}
	p := rpc.LauncherEnqueueParams{Workspace: repo, Input: input, Harness: "claude"}
	if err := live.c.Call(ctx, rpc.MethodLauncherEnqueue, p, nil); err != nil {
		t.Fatal(err)
	}

	snapshot := func() rpc.State {
		sub, err := dial(t, live.path).Subscribe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return sub.State
	}
	paneOf := func(ref string) string {
		st := snapshot()
		for _, task := range st.Tasks {
			if task.Ref != ref {
				continue
			}
			for _, s := range st.Sessions {
				if s.TaskID == task.ID {
					return s.Pane
				}
			}
		}
		return ""
	}
	waitFor(t, func() bool { return len(snapshot().Sessions) == 3 && len(snapshot().Queue) == 2 })
	time.Sleep(300 * time.Millisecond)
	if st := snapshot(); len(st.Sessions) != 3 || len(st.Queue) != 2 {
		t.Fatalf("%d sessions and %d queued, want 3 and 2", len(st.Sessions), len(st.Queue))
	}
	for _, slug := range []string{"eng-1", "eng-2", "eng-3"} {
		if _, err := os.Stat(filepath.Join(home, "worktrees", "api", slug)); err != nil {
			t.Fatalf("no worktree for %s: %v", slug, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "worktrees", "api", "eng-4")); !os.IsNotExist(err) {
		t.Fatalf("a queued issue has a worktree: %v", err)
	}

	for n, finish := range []string{"ENG-1", "ENG-2"} {
		pane := paneOf(finish)
		if pane == "" {
			t.Fatalf("no pane for %s", finish)
		}
		if err := live.c.Call(ctx, rpc.MethodHook, rpc.Hook{Harness: "claude", Event: "Stop", Pane: pane}, nil); err != nil {
			t.Fatal(err)
		}
		want := 4 + n
		waitFor(t, func() bool { return len(snapshot().Sessions) == want })
	}
	waitFor(t, func() bool { return len(snapshot().Queue) == 0 })

	waitFor(t, func() bool {
		st := snapshot()
		named := 0
		for _, task := range st.Tasks {
			if domain.NameFor(task, nil) == "Title of "+task.Ref {
				named++
			}
		}
		return named == 5
	})
}

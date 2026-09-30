//go:build integration

package daemon_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitadapter "github.com/giovaniif/agent-workspace/internal/adapters/git"
	"github.com/giovaniif/agent-workspace/internal/adapters/tmux"
	"github.com/giovaniif/agent-workspace/internal/daemon"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// TestReviewSendReachesARealAgentPaneAcrossARestart comments on two files
// of a real worktree, restarts the daemon, sends the draft, and reads what
// a fake agent running in a real tmux pane received.
func TestReviewSendReachesARealAgentPaneAcrossARestart(t *testing.T) {
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
	received := filepath.Join(home, "received")
	fakeAgent := filepath.Join(home, "fake-agent")
	script := fmt.Sprintf("#!/bin/sh\nstty -echo\nexec cat > %s\n", received)
	if err := os.WriteFile(fakeAgent, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("agentws-it-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		_ = tmux.New(tmux.Config{Socket: socket, ConfigPath: filepath.Join(home, "tmux.conf")}).Close(context.Background())
	})
	review := []daemon.Option{daemon.WithReview(gitadapter.Review{}), daemon.WithHunks(gitadapter.Review{})}
	live := serveLive(t, home, socket, fakeAgent, review...)
	ctx := context.Background()
	if err := live.c.Call(ctx, rpc.MethodWorkspaceAdd, rpc.WorkspaceAddParams{Path: repo}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := live.c.OpenClient(ctx, rpc.OpenClientParams{Command: []string{"cat"}}); err != nil {
		t.Fatal(err)
	}
	var s domain.Session
	if err := live.c.Call(ctx, rpc.MethodNewSession, rpc.NewSessionParams{Workspace: repo, WorkItem: "https://github.com/acme/api/pull/42", Harness: "claude"}, &s); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(home, "worktrees", "api", "api-42")
	if err := os.WriteFile(filepath.Join(wt, "README"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var rv rpc.Review
	waitFor(t, func() bool {
		rv, err = live.c.Review(ctx, rpc.ReviewParams{Session: s.ID, Scope: domain.ScopeUncommitted})
		return err == nil && len(rv.Worktrees) == 1 && rv.Worktrees[0].Worktree.Path == wt && len(rv.Worktrees[0].Files) == 2
	})
	var want []domain.ReviewComment
	for _, f := range rv.Worktrees[0].Files {
		c := domain.CommentOn(wt, f.Path, f.Hunks[0].Lines[len(f.Hunks[0].Lines)-1:], "look at "+f.Path)
		want = append(want, c)
		if _, err := live.c.AddReviewComment(ctx, s.ID, c); err != nil {
			t.Fatal(err)
		}
	}

	live.cancel()
	live = serveLive(t, home, socket, fakeAgent, review...)
	rv, err = live.c.Review(ctx, rpc.ReviewParams{Session: s.ID, Scope: domain.ScopeUncommitted})
	if err != nil || len(rv.Draft.Comments) != 2 {
		t.Fatalf("after a restart the draft is %+v, %v", rv.Draft, err)
	}
	sent, err := live.c.SendReview(ctx, s.ID)
	if err != nil || sent.Status != domain.DraftSent {
		t.Fatalf("send = %+v, %v", sent, err)
	}
	var got string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(received)
		if got = string(b); strings.Contains(got, "look at main.go") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, line := range strings.Split(strings.TrimSpace(domain.ReviewPrompt(want)), "\n") {
		if !strings.Contains(got, line) {
			t.Errorf("the agent did not receive %q; got:\n%s", line, got)
		}
	}
}

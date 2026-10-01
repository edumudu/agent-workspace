package daemon_test

import (
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/daemon"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestAgentPanesGetATitleThatFollowsTheirSession(t *testing.T) {
	store := &memStore{}
	store.snap.Sessions = []domain.Session{{ID: "a", Harness: domain.HarnessClaude, Pane: "%3", State: domain.StateIdle, WorktreeIDs: []string{"w"}}}
	store.snap.Worktrees = []domain.Worktree{{ID: "w", Repo: "/src/api", SubtaskSlug: "x", SessionID: "a"}}
	r := startSessions(t, store, nil)
	waitUntil(t, "the title to be set", func() bool { return r.host.title("%3") == "○ claude │ api:x no PR" })

	r.d.Post(daemon.SessionChanged{Session: domain.Session{ID: "a", Harness: domain.HarnessClaude, Pane: "%3", State: domain.StateRunning, WorktreeIDs: []string{"w"}}})
	waitUntil(t, "the title to follow the state", func() bool { return r.host.title("%3") == "◐ claude │ api:x no PR" })
	if n := r.host.titleSets(app.PaneID("%3")); n != 2 {
		t.Fatalf("title set %d times; want once per change", n)
	}
}

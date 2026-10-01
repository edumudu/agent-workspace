package daemon_test

import (
	"context"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

func (r *attentionRig) removal(t *testing.T) string {
	t.Helper()
	select {
	case g := <-r.n.removals:
		return g
	case <-time.After(2 * time.Second):
		t.Fatal("no removal")
	}
	return ""
}

func TestNotifyBannerIsGroupedBySession(t *testing.T) {
	r := newRig(t, &memStore{}, nil)
	r.addRunning(t, "s1", domain.HarnessClaude, "%1")
	r.hook(t, "claude", "Stop", "%1", "")
	if got := r.banner(t); got.Group != "s1" {
		t.Fatalf("group %q", got.Group)
	}
}

func TestNotifyBannerActivatesTheClientTerminal(t *testing.T) {
	r := newRig(t, &memStore{}, nil)
	r.d.SetClientHost(&fakeClientHost{})
	if _, err := r.c.OpenClient(context.Background(), rpc.OpenClientParams{Command: []string{"agentws", "tui"}, Terminal: "com.apple.Terminal"}); err != nil {
		t.Fatal(err)
	}
	r.addRunning(t, "s1", domain.HarnessClaude, "%1")
	r.hook(t, "claude", "Stop", "%1", "")
	if got := r.banner(t); got.Terminal != "com.apple.Terminal" {
		t.Fatalf("terminal %q", got.Terminal)
	}
}

func TestNotifyResumingWithdrawsTheBanner(t *testing.T) {
	r := newRig(t, &memStore{}, nil)
	r.addRunning(t, "s1", domain.HarnessClaude, "%1")
	r.hook(t, "claude", "PermissionRequest", "%1", "")
	r.banner(t)
	r.hook(t, "claude", "PreToolUse", "%1", `{"tool_name":"Bash"}`)
	if got := r.removal(t); got != "s1" {
		t.Fatalf("removed %q", got)
	}
}

func TestNotifyFocusingWithdrawsTheBanner(t *testing.T) {
	r := newRig(t, &memStore{}, nil)
	r.addRunning(t, "s1", domain.HarnessClaude, "%1")
	r.hook(t, "claude", "Stop", "%1", "")
	r.banner(t)
	if err := r.c.FocusSession(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if got := r.removal(t); got != "s1" {
		t.Fatalf("removed %q", got)
	}
}

func TestNotifyNothingToWithdrawWithoutABanner(t *testing.T) {
	r := newRig(t, &memStore{}, nil)
	r.addRunning(t, "s1", domain.HarnessClaude, "%1")
	if err := r.c.FocusSession(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	next(t, r.sub.Diffs)
	r.hook(t, "claude", "PreToolUse", "%1", `{"tool_name":"Bash"}`)
	r.addRunning(t, "s2", domain.HarnessClaude, "%2")
	r.hook(t, "claude", "Stop", "%2", "")
	r.banner(t)
	select {
	case g := <-r.n.removals:
		t.Fatalf("removed %q with no banner up", g)
	default:
	}
}

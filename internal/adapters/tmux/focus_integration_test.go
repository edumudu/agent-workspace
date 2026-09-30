//go:build integration

package tmux_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/adapters/tmux"
	"github.com/giovaniif/agent-workspace/internal/app"
)

func tmuxOn(t testing.TB, attach []string, args ...string) string {
	t.Helper()
	full := append(slices.Clone(attach[1:len(attach)-3]), args...)
	out, err := exec.Command("tmux", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v: %v: %s", full, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestFocusKeyReturnsFromTheAgentPaneToTheSidebar(t *testing.T) {
	ctx := context.Background()
	h := newHost(t)
	slot, err := h.OpenClient(ctx, "main", app.PaneSpec{Name: "tui", Command: []string{"sleep", "600"}})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := h.Create(ctx, catPane("agent"))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Show(ctx, agent, slot); err != nil {
		t.Fatal(err)
	}
	if err := h.FocusSlot(ctx, slot); err != nil {
		t.Fatal(err)
	}
	attach := h.AttachCommand(slot)
	activeIndex := func() string {
		return tmuxOn(t, attach, "display-message", "-p", "-t", string(slot), "#{pane_index}")
	}
	if got := activeIndex(); got != "1" {
		t.Fatalf("active pane = %s before the key; want the agent pane, 1", got)
	}

	outer := fmt.Sprintf("agentws-outer-%d", os.Getpid())
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", outer, "kill-server").Run() })
	terminal := []string{"-L", outer, "-f", "/dev/null", "new-session", "-d", "-x", "200", "-y", "50", strings.Join(attach, " ")}
	if out, err := exec.Command("tmux", terminal...).CombinedOutput(); err != nil {
		t.Fatalf("terminal: %v: %s", err, out)
	}
	waitFor(t, "a terminal to attach", func() bool {
		return tmuxOn(t, attach, "list-clients") != ""
	})
	if out, err := exec.Command("tmux", "-L", outer, "send-keys", "-t", "0", tmux.FocusSidebarKey).CombinedOutput(); err != nil {
		t.Fatalf("send-keys: %v: %s", err, out)
	}
	waitFor(t, "focus to return to the sidebar", func() bool { return activeIndex() == "0" })

	if out, _ := exec.Command("tmux", "list-keys", "-T", "root").CombinedOutput(); strings.Contains(string(out), "select-pane -t :.0") {
		t.Fatalf("the default tmux server got the binding:\n%s", out)
	}
}

func TestEnsureSlotGivesAnEmptySlotAnEmptyStatePane(t *testing.T) {
	ctx := context.Background()
	h := newHost(t)
	slot, err := h.OpenClient(ctx, "main", app.PaneSpec{Name: "tui", Command: []string{"sleep", "600"}})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := h.Create(ctx, catPane("agent"))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Show(ctx, agent, slot); err != nil {
		t.Fatal(err)
	}
	if err := h.Kill(ctx, agent); err != nil {
		t.Fatal(err)
	}
	if err := h.EnsureSlot(ctx, slot); err != nil {
		t.Fatal(err)
	}
	if err := h.EnsureSlot(ctx, slot); err != nil {
		t.Fatalf("second EnsureSlot: %v", err)
	}
	pane := h.ShownIn(ctx, slot)
	if pane == "" {
		t.Fatal("the slot has no pane after EnsureSlot")
	}
	waitFor(t, "the empty-state text", func() bool {
		out, err := h.Capture(ctx, pane, 20)
		return err == nil && strings.Contains(out, "No session in view")
	})
	got := tmuxOn(t, h.AttachCommand(slot), "list-panes", "-t", string(slot), "-F", "#{pane_id}")
	if len(strings.Fields(got)) != 2 {
		t.Fatalf("panes in the client window = %q; want the sidebar and one slot pane", got)
	}
}

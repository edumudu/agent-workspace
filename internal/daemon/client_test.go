package daemon_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

type fakeClientHost struct {
	mu      sync.Mutex
	opened  []app.PaneSpec
	open    map[app.Slot]bool
	focused []app.Slot
}

func (h *fakeClientHost) OpenClient(_ context.Context, name string, tui app.PaneSpec) (app.Slot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.opened = append(h.opened, tui)
	slot := app.Slot("@" + name + string(rune('0'+len(h.opened))))
	if h.open == nil {
		h.open = map[app.Slot]bool{}
	}
	h.open[slot] = true
	return slot, nil
}

func (h *fakeClientHost) ClientOpen(_ context.Context, slot app.Slot) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.open[slot]
}

func (h *fakeClientHost) AttachCommand(slot app.Slot) []string {
	return []string{"tmux", "attach", "-t", string(slot)}
}

func (h *fakeClientHost) FocusSlot(_ context.Context, slot app.Slot) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.open[slot] {
		return errors.New("no such slot")
	}
	h.focused = append(h.focused, slot)
	return nil
}

func (h *fakeClientHost) close(slot app.Slot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.open, slot)
}

func TestOpenClientCreatesTheLayoutOnceAndReattachesAfter(t *testing.T) {
	d, path := start(t, &memStore{})
	host := &fakeClientHost{}
	d.SetClientHost(host)
	c := dial(t, path)
	ctx := context.Background()
	tui := rpc.OpenClientParams{Command: []string{"agentws", "tui"}}

	first, err := c.OpenClient(ctx, tui)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.OpenClient(ctx, tui)
	if err != nil {
		t.Fatal(err)
	}
	if first.Slot != second.Slot || len(host.opened) != 1 {
		t.Fatalf("slots %q, %q after %d opens; want one layout reused", first.Slot, second.Slot, len(host.opened))
	}
	if !slices.Equal(first.Attach, []string{"tmux", "attach", "-t", first.Slot}) {
		t.Fatalf("attach = %v", first.Attach)
	}
	if got := host.opened[0]; !slices.Equal(got.Command, tui.Command) {
		t.Fatalf("tui pane spec = %+v; want the TUI command", got)
	}

	host.close(app.Slot(first.Slot))
	third, err := c.OpenClient(ctx, tui)
	if err != nil {
		t.Fatal(err)
	}
	if third.Slot == first.Slot || len(host.opened) != 2 {
		t.Fatalf("after the layout closed got slot %q, %d opens; want a new layout", third.Slot, len(host.opened))
	}
}

func TestFocusMainMovesFocusToTheSlot(t *testing.T) {
	d, path := start(t, &memStore{})
	host := &fakeClientHost{}
	d.SetClientHost(host)
	c := dial(t, path)
	ctx := context.Background()
	var rerr *rpc.Error
	if err := c.FocusMain(ctx); !errors.As(err, &rerr) {
		t.Fatalf("focus before any layout = %v; want an rpc error", err)
	}
	opened, err := c.OpenClient(ctx, rpc.OpenClientParams{Command: []string{"agentws", "tui"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.FocusMain(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(host.focused, []app.Slot{app.Slot(opened.Slot)}) {
		t.Fatalf("focused = %v", host.focused)
	}
}

func TestClientMethodsWithoutATerminalHostFail(t *testing.T) {
	_, path := start(t, &memStore{})
	var rerr *rpc.Error
	_, err := dial(t, path).OpenClient(context.Background(), rpc.OpenClientParams{Command: []string{"agentws", "tui"}})
	if !errors.As(err, &rerr) || rerr.Code != rpc.CodeUnavailable {
		t.Fatalf("err = %v; want %s", err, rpc.CodeUnavailable)
	}
}

func TestDebugSeedAddsFakeSessionsWithWorktrees(t *testing.T) {
	_, path := start(t, &memStore{})
	c := dial(t, path)
	ctx := context.Background()
	if err := c.DebugSeed(ctx, 3); err != nil {
		t.Fatal(err)
	}
	sub, err := dial(t, path).Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := sub.State
	if len(st.Sessions) != 3 || len(st.Tasks) == 0 || len(st.Worktrees) < 3 {
		t.Fatalf("seeded %d sessions, %d tasks, %d worktrees", len(st.Sessions), len(st.Tasks), len(st.Worktrees))
	}
	tasks := map[string]bool{}
	for _, task := range st.Tasks {
		tasks[task.ID] = true
	}
	worktrees := map[string]bool{}
	for _, w := range st.Worktrees {
		worktrees[w.ID] = true
	}
	for _, s := range st.Sessions {
		if !tasks[s.TaskID] {
			t.Errorf("session %s has unknown task %s", s.ID, s.TaskID)
		}
		for _, id := range s.WorktreeIDs {
			if !worktrees[id] {
				t.Errorf("session %s has unknown worktree %s", s.ID, id)
			}
		}
	}
	if err := c.DebugSeed(ctx, 0); err == nil {
		t.Fatal("seeding 0 sessions succeeded")
	}
}

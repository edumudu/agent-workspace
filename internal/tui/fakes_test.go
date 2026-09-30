package tui_test

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

type call struct {
	method string
	params any
}

// fakeCaller answers session.new with a session on pane %5 and every other
// method with nothing.
type fakeCaller struct {
	mu    sync.Mutex
	calls []call
	err   error
}

func (f *fakeCaller) Call(_ context.Context, method string, params, out any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{method, params})
	if f.err != nil {
		return f.err
	}
	if method == rpc.MethodNewSession {
		if s, ok := out.(*domain.Session); ok {
			b, _ := json.Marshal(domain.Session{ID: "new1", Pane: "%5", State: domain.StateIdle})
			return json.Unmarshal(b, s)
		}
	}
	return nil
}

type fakeAttender struct {
	muted   []mute
	focused []string
}

type mute struct {
	id    string
	muted bool
}

func (f *fakeAttender) MuteSession(_ context.Context, id string, muted bool) error {
	f.muted = append(f.muted, mute{id, muted})
	return nil
}

func (f *fakeAttender) FocusSession(_ context.Context, id string) error {
	f.focused = append(f.focused, id)
	return nil
}

type fakeKiller struct {
	killed [][]int
	err    error
}

func (f *fakeKiller) KillPorts(_ context.Context, pgids []int) ([]int, error) {
	f.killed = append(f.killed, pgids)
	return pgids, f.err
}

type fakeFocuser struct{ calls int }

func (f *fakeFocuser) FocusMain(context.Context) error {
	f.calls++
	return nil
}

func (f *fakeCaller) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.method)
	}
	return out
}

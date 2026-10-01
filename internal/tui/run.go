package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const TickInterval = 200 * time.Millisecond

// why: at 60 a keypress can wait up to 16 ms for the next frame on its own, so
// the renderer runs at its 120 maximum.
const FPS = 120

// why: diffs are read on a second connection so a slow frame never delays
// FocusMain.
func Run(ctx context.Context, subscriber, caller *rpc.Client, opts Options) error {
	sub, err := subscriber.Subscribe(ctx)
	if err != nil {
		return err
	}
	opts.Tick = TickInterval
	opts.Focus, opts.Attend, opts.Kill, opts.Calls = caller, caller, caller, caller
	opts.Switch, opts.Review, opts.Disk = caller, caller, caller
	m := New(opts)
	next, _ := m.Update(StateMsg(sub.State))
	p := tea.NewProgram(next, tea.WithContext(ctx), tea.WithFPS(FPS))
	go func() {
		for d := range sub.Diffs {
			p.Send(DiffMsg(d))
		}
		p.Send(DisconnectedMsg{})
	}()
	_, err = p.Run()
	return err
}

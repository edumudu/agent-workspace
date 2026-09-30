package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// TickInterval drives the running spinner and the clock.
const TickInterval = 200 * time.Millisecond

// FPS is the renderer's frame cap. At 60 a keypress can wait up to 16 ms for
// the next frame on its own, so the renderer runs at its 120 maximum.
const FPS = 120

// Run subscribes to the daemon and runs the TUI until the user quits or ctx
// ends. Diffs are read on a second connection so a slow frame never delays
// FocusMain.
func Run(ctx context.Context, subscriber, caller *rpc.Client, theme Theme, defaults map[domain.Harness]Defaults) error {
	sub, err := subscriber.Subscribe(ctx)
	if err != nil {
		return err
	}
	m := New(Options{Theme: theme, Tick: TickInterval, Focus: caller, Attend: caller, Kill: caller, Calls: caller, Switch: caller, Defaults: defaults, Review: caller})
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

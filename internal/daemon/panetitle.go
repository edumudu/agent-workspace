package daemon

import (
	"context"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

// titleEvery is how often agent pane titles are brought up to date.
const titleEvery = 250 * time.Millisecond

// paneTitles keeps each agent pane's top border in step with its session
// (ADR 0038). It reads state on the loop and runs tmux off it, only for
// titles that changed.
func (d *Daemon) paneTitles(ctx context.Context) {
	if d.hs.host == nil {
		return
	}
	set := map[app.PaneID]string{}
	tick, stop := ticker(titleEvery)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		want := map[app.PaneID]string{}
		ok := d.query(func(s *state) {
			wts := sorted(s.worktrees)
			for _, x := range s.sessions {
				if x.Pane != "" {
					want[app.PaneID(x.Pane)] = domain.AgentTitle(x, wts, s.hints.cwd[x.ID])
				}
			}
		})
		if !ok {
			continue
		}
		for pane, title := range want {
			if set[pane] != title && d.hs.host.SetTitle(ctx, pane, title) == nil {
				set[pane] = title
			}
		}
		for pane := range set {
			if _, live := want[pane]; !live {
				delete(set, pane)
			}
		}
	}
}

package app

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// ReconcilePanes returns every session whose pane no longer exists or has
// died, ended. Sessions already off a pane are skipped. It ends nothing if
// the host cannot be listed.
func ReconcilePanes(ctx context.Context, host TerminalHost, sessions []domain.Session) ([]domain.Session, error) {
	panes, err := host.List(ctx)
	if err != nil {
		return nil, err
	}
	alive := make(map[PaneID]bool, len(panes))
	for _, p := range panes {
		alive[p.ID] = p.Alive
	}
	var ended []domain.Session
	for _, s := range sessions {
		if s.Pane == "" || alive[PaneID(s.Pane)] {
			continue
		}
		ended = append(ended, s.End())
	}
	return ended, nil
}

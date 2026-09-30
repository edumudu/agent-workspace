package app

import "context"

// PaneBinding and PaneBindingStore are the narrow slice of the session store
// that reconcile needs; the full Store port replaces them once it exists.
type PaneBinding struct {
	SessionID string
	Pane      PaneID
}

type PaneBindingStore interface {
	PaneBindings(ctx context.Context) ([]PaneBinding, error)
	MarkIdle(ctx context.Context, sessionID string) error
}

// ReconcilePanes idles every session whose pane no longer exists or has died,
// and returns their IDs. It changes nothing if the host cannot be listed.
func ReconcilePanes(ctx context.Context, host TerminalHost, store PaneBindingStore) ([]string, error) {
	panes, err := host.List(ctx)
	if err != nil {
		return nil, err
	}
	bindings, err := store.PaneBindings(ctx)
	if err != nil {
		return nil, err
	}
	alive := make(map[PaneID]bool, len(panes))
	for _, p := range panes {
		alive[p.ID] = p.Alive
	}
	var idled []string
	for _, b := range bindings {
		if alive[b.Pane] {
			continue
		}
		if err := store.MarkIdle(ctx, b.SessionID); err != nil {
			return idled, err
		}
		idled = append(idled, b.SessionID)
	}
	return idled, nil
}

package app

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// why: Listeners runs external commands, so callers keep it off the event
// loop and off render paths.
type ProcessTable interface {
	Listeners(ctx context.Context) ([]domain.Listener, error)
	Terminate(ctx context.Context, pgid int) error
}

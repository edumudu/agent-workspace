package app

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// ProcessTable reads and ends processes. Listeners runs external commands, so
// callers keep it off the event loop and off render paths.
type ProcessTable interface {
	// Listeners lists TCP listen sockets with the owning process's group and
	// cwd. Cwd is empty for a process whose cwd could not be read.
	Listeners(ctx context.Context) ([]domain.Listener, error)
	// Terminate asks the process group to exit, and kills it if it has not
	// after a grace period. It returns once the group is gone.
	Terminate(ctx context.Context, pgid int) error
}

package app

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// Notifier shows one banner. It may run a process, so callers never use it
// on the daemon loop.
type Notifier interface {
	Notify(ctx context.Context, b domain.Banner) error
}

// Foreground answers whether a terminal app has the user's attention now.
type Foreground interface {
	TerminalFrontmost(ctx context.Context) bool
}

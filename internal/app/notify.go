package app

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// why: it may run a process, so callers never use it on the daemon loop.
type Notifier interface {
	Notify(ctx context.Context, b domain.Banner) error
	// why: a backend that cannot withdraw banners does nothing.
	Remove(ctx context.Context, group string) error
}

type Foreground interface {
	TerminalFrontmost(ctx context.Context) bool
}

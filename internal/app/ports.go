package app

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

type ProcessTable interface {
	Listeners(ctx context.Context) ([]domain.Listener, error)
	Terminate(ctx context.Context, pgid int) error
}

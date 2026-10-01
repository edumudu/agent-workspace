package app

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// why: every method reads or writes config files, so callers keep it off the event loop and off render paths.
type Onboarder interface {
	Onboarding(ctx context.Context) (domain.Onboarding, error)
	Install(ctx context.Context, h domain.Harness) (domain.HarnessSetup, error)
	Finish(ctx context.Context) error
}

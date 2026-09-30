//go:build integration

package daemon_test

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

type noPRs struct{}

func (noPRs) PRs(context.Context, string) ([]domain.PullRequest, error) { return nil, nil }

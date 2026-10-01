package tui_test

import (
	"context"
	"sync"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

type fakeOnboarder struct {
	mu        sync.Mutex
	state     domain.Onboarding
	installed []domain.Harness
	finished  int
	err       error
}

func (f *fakeOnboarder) Onboarding(context.Context) (domain.Onboarding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}

func (f *fakeOnboarder) OnboardInstall(_ context.Context, h domain.Harness) (domain.HarnessSetup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return domain.HarnessSetup{}, f.err
	}
	f.installed = append(f.installed, h)
	s := &f.state.Claude
	if h == domain.HarnessCodex {
		s = &f.state.Codex
	}
	s.Installed = true
	return *s, nil
}

func (f *fakeOnboarder) OnboardFinish(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished++
	f.state.Done = true
	return nil
}

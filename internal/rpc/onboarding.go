package rpc

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

const (
	MethodOnboarding        = "onboarding.status"
	MethodOnboardingInstall = "onboarding.install"
	MethodOnboardingFinish  = "onboarding.finish"
)

type OnboardInstallParams struct {
	Harness domain.Harness `json:"harness"`
}

func (c *Client) Onboarding(ctx context.Context) (domain.Onboarding, error) {
	var out domain.Onboarding
	err := c.Call(ctx, MethodOnboarding, nil, &out)
	return out, err
}

func (c *Client) OnboardInstall(ctx context.Context, h domain.Harness) (domain.HarnessSetup, error) {
	var out domain.HarnessSetup
	err := c.Call(ctx, MethodOnboardingInstall, OnboardInstallParams{Harness: h}, &out)
	return out, err
}

func (c *Client) OnboardFinish(ctx context.Context) error {
	return c.Call(ctx, MethodOnboardingFinish, nil, nil)
}

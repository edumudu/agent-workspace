package app

import "github.com/giovaniif/agent-workspace/internal/domain"

type LaunchRequest struct {
	Name   string
	Dir    string
	Model  string
	Effort string
	Prompt string
	Resume string
}

type HarnessAdapter interface {
	Harness() domain.Harness
	Launch(req LaunchRequest) PaneSpec
}

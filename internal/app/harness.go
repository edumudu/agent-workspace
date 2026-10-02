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

// why: the pane's $TMUX_PANE is how the session's hooks find their way back.
type HarnessAdapter interface {
	Harness() domain.Harness
	Launch(req LaunchRequest) PaneSpec
}

package app

import "github.com/giovaniif/agent-workspace/internal/domain"

// LaunchRequest is what a new session asks its harness for. Empty Model and
// Effort leave the harness defaults; Prompt, when set, is the first prompt.
type LaunchRequest struct {
	Name   string
	Dir    string
	Model  string
	Effort string
	Prompt string
}

// HarnessAdapter turns a launch request into the pane that runs the harness.
// The pane's $TMUX_PANE is how the session's hooks find their way back to it.
type HarnessAdapter interface {
	Harness() domain.Harness
	Launch(req LaunchRequest) PaneSpec
}

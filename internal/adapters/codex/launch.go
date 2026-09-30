package codex

import (
	"context"
	"fmt"

	"github.com/giovaniif/agent-workspace/internal/app"
)

type LaunchRequest struct {
	Name   string
	Dir    string
	Model  string
	Effort string
	Prompt string
}

func LaunchSpec(req LaunchRequest) app.PaneSpec {
	command := []string{"codex"}
	if req.Model != "" {
		command = append(command, "--model", req.Model)
	}
	if req.Effort != "" {
		command = append(command, "-c", fmt.Sprintf("model_reasoning_effort=%q", req.Effort))
	}
	if req.Prompt != "" {
		// why: the separator keeps a prompt that starts with "-" from parsing as a flag.
		command = append(command, "--", req.Prompt)
	}
	return app.PaneSpec{Name: req.Name, Dir: req.Dir, Command: command}
}

// Launch starts codex in a new pane. The pane's $TMUX_PANE is how the
// session's hooks find their way back to it.
func Launch(ctx context.Context, host app.TerminalHost, req LaunchRequest) (app.PaneID, error) {
	return host.Create(ctx, LaunchSpec(req))
}

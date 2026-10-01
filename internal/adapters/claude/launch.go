package claude

import (
	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

type Adapter struct {
	Binary string
}

func (Adapter) Harness() domain.Harness { return domain.HarnessClaude }

func (a Adapter) Launch(req app.LaunchRequest) app.PaneSpec {
	bin := a.Binary
	if bin == "" {
		bin = "claude"
	}
	command := []string{bin}
	if req.Model != "" {
		command = append(command, "--model", req.Model)
	}
	if req.Effort != "" {
		command = append(command, "--effort", req.Effort)
	}
	if req.Prompt != "" {
		// why: the separator keeps a prompt that starts with "-" from parsing as a flag.
		command = append(command, "--", req.Prompt)
	}
	return app.PaneSpec{Name: req.Name, Dir: req.Dir, Command: command}
}

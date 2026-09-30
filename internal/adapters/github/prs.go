// Package github implements app.PRFinder with the gh CLI, so it reuses the
// user's gh auth.
package github

import (
	"context"
	"encoding/json"
	"os/exec"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

var _ app.PRFinder = Finder{}

// Finder runs Bin ("gh" when empty).
type Finder struct {
	Bin string
}

// PRs runs one `gh pr list` in repo for its 100 most recent PRs in any state.
func (f Finder) PRs(ctx context.Context, repo string) ([]domain.PullRequest, error) {
	bin := f.Bin
	if bin == "" {
		bin = "gh"
	}
	cmd := exec.CommandContext(ctx, bin, "pr", "list", "--state", "all", "--limit", "100",
		"--json", "number,title,url,headRefName,state,statusCheckRollup")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parsePRs(out)
}

type ghPR struct {
	Number int       `json:"number"`
	Title  string    `json:"title"`
	URL    string    `json:"url"`
	Head   string    `json:"headRefName"`
	State  string    `json:"state"`
	Checks []ghCheck `json:"statusCheckRollup"`
}

// ghCheck is a CheckRun (Status, Conclusion) or a StatusContext (State).
type ghCheck struct {
	Typename   string `json:"__typename"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

func parsePRs(out []byte) ([]domain.PullRequest, error) {
	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	prs := make([]domain.PullRequest, len(raw))
	for i, p := range raw {
		states := make([]domain.CheckState, len(p.Checks))
		for j, c := range p.Checks {
			states[j] = c.state()
		}
		prs[i] = domain.PullRequest{
			Number: p.Number, Title: p.Title, URL: p.URL, Head: p.Head,
			State: domain.PRState(p.State), Checks: domain.RollupChecks(states),
		}
	}
	return prs, nil
}

func (c ghCheck) state() domain.CheckState {
	if c.Typename == "StatusContext" {
		switch c.State {
		case "SUCCESS":
			return domain.CheckPassing
		case "FAILURE", "ERROR":
			return domain.CheckFailing
		default:
			return domain.CheckPending
		}
	}
	if c.Status != "COMPLETED" {
		return domain.CheckPending
	}
	switch c.Conclusion {
	case "SUCCESS":
		return domain.CheckPassing
	case "NEUTRAL", "SKIPPED":
		return domain.CheckNone
	default:
		return domain.CheckFailing
	}
}

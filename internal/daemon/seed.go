package daemon

import (
	"fmt"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

var seedStates = []domain.AgentState{
	domain.StateRunning, domain.StateWaiting, domain.StateDone,
	domain.StateIdle, domain.StatePermission,
}

var seedRepos = []string{"api", "web", "infra"}

// seed makes n fake sessions, two per task, each with one to three
// worktrees. IDs are prefixed "seed-" so a second seed replaces the first.
func seed(n int) []Event {
	var events []Event
	for i := range n {
		taskID := fmt.Sprintf("seed-task-%d", i/2+1)
		if i%2 == 0 {
			events = append(events, TaskChanged{Task: domain.Task{
				ID: taskID, Source: domain.TaskText, Ref: fmt.Sprintf("#%d", 40+i/2),
				Text: fmt.Sprintf("seeded task %d", i/2+1),
			}})
		}
		s := domain.Session{
			ID:      fmt.Sprintf("seed-session-%d", i+1),
			TaskID:  taskID,
			Harness: domain.HarnessClaude,
			Model:   "opus-5.5",
			Effort:  "high",
			State:   seedStates[i%len(seedStates)],
			Usage:   domain.Usage{ContextLeftPercent: 90 - (i*13)%70},
		}
		if i%3 == 1 {
			s.Harness, s.Model, s.Effort = domain.HarnessCodex, "gpt-6", "med"
		}
		s.Unread = s.State == domain.StateDone
		for w := range i%3 + 1 {
			repo := seedRepos[w]
			wt := domain.Worktree{
				ID:          fmt.Sprintf("seed-wt-%d-%d", i+1, w+1),
				Repo:        repo,
				Path:        fmt.Sprintf("/tmp/seed/%s-%d", repo, i+1),
				Branch:      fmt.Sprintf("seed-%d-%s", i+1, repo),
				SubtaskSlug: fmt.Sprintf("part-%d", w+1),
			}
			if w == 0 {
				wt.PR = &domain.PullRequest{Number: 100 + i, Title: fmt.Sprintf("seeded change %d", i+1)}
			}
			events = append(events, WorktreeChanged{Worktree: wt})
			s.WorktreeIDs = append(s.WorktreeIDs, wt.ID)
		}
		events = append(events, SessionChanged{Session: s})
	}
	return events
}

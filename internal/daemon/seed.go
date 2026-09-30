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
		for _, ev := range seedLog(s) {
			events = append(events, SessionHooked{Session: s, Event: ev})
		}
	}
	return events
}

func seedLog(s domain.Session) []domain.SessionEvent {
	log := []domain.SessionEvent{
		{Kind: domain.EventUserPromptSubmit},
		{Kind: domain.EventPreToolUse, Tool: "Read", Detail: "internal/upload.go"},
		{Kind: domain.EventPreToolUse, Tool: "Edit", Detail: "internal/upload.go"},
		{Kind: domain.EventPreToolUse, Tool: "Bash", Detail: "go test ./..."},
	}
	switch s.State {
	case domain.StatePermission:
		log = append(log, domain.SessionEvent{Kind: domain.EventPermissionRequest, Tool: "Bash",
			Text: "Bash: rm -rf build\nmake clean\nmake all\nmake install"})
	case domain.StateWaiting:
		log = append(log,
			domain.SessionEvent{Kind: domain.EventStop, Text: "Tests pass.\n\nWant me to open the PR now, or wait for review?"},
			domain.SessionEvent{Kind: domain.EventWaitingForInput, Text: "Claude is waiting for your input"})
	case domain.StateDone:
		log = append(log, domain.SessionEvent{Kind: domain.EventStop, Text: "Done. The upload retries three times."})
	}
	for i := range log {
		log[i].SessionID = s.ID
	}
	return log
}

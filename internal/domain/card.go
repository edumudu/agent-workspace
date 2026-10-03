package domain

import (
	"path"
	"strings"
)

const (
	CardActions      = 3
	CardWaitingLines = 3
	CardSaidLines    = 4
)

type SessionCard struct {
	Title   string
	Name    string
	Ref     string
	PRs     []PullRequest
	Actions []string
	Waiting string
	Said    string
}

func BuildSessionCard(task Task, session Session, worktrees []Worktree, events []SessionEvent) SessionCard {
	prs := pullRequests(worktrees)
	card := SessionCard{Title: NameFor(task, nil), Name: NameFor(task, prs), Ref: task.Ref, PRs: prs}
	var own []SessionEvent
	for _, ev := range events {
		if ev.SessionID == session.ID {
			own = append(own, ev)
		}
	}
	card.Actions = lastActions(own)
	card.Waiting = waitingOn(session.State, own)
	if ev, ok := latest(own, EventStop); ok {
		card.Said = cutLines(ev.Text, CardSaidLines)
	}
	return card
}

func pullRequests(worktrees []Worktree) []PullRequest {
	var prs []PullRequest
	seen := map[int]bool{}
	for _, w := range worktrees {
		if w.PR == nil || seen[w.PR.Number] {
			continue
		}
		seen[w.PR.Number] = true
		prs = append(prs, *w.PR)
	}
	return prs
}

func lastActions(events []SessionEvent) []string {
	var out []string
	for i := len(events) - 1; i >= 0 && len(out) < CardActions; i-- {
		ev := events[i]
		if ev.Kind != EventPreToolUse {
			continue
		}
		out = append(out, actionLabel(ev.Tool, ev.Detail))
	}
	return out
}

var fileTools = map[string]bool{"Read": true, "Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true}

func actionLabel(tool, detail string) string {
	switch {
	case detail == "":
		return tool
	case tool == "Bash":
		return detail
	case fileTools[tool]:
		return tool + " " + path.Base(detail)
	}
	return tool + ": " + detail
}

func waitingOn(state AgentState, events []SessionEvent) string {
	turn := sinceLastPrompt(events)
	switch state {
	case StatePermission:
		if ev, ok := latest(turn, EventPermissionRequest); ok {
			return cutLines(ev.Text, CardWaitingLines)
		}
	case StateWaiting, StateDone:
		if q := lastQuestion(turn); q != "" {
			return cutLines(q, CardWaitingLines)
		}
		if ev, ok := latest(turn, EventWaitingForInput); ok && state == StateWaiting {
			return cutLines(ev.Text, CardWaitingLines)
		}
	}
	return ""
}

func sinceLastPrompt(events []SessionEvent) []SessionEvent {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == EventUserPromptSubmit {
			return events[i+1:]
		}
	}
	return events
}

func latest(events []SessionEvent, kind HarnessEventKind) (SessionEvent, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == kind {
			return events[i], true
		}
	}
	return SessionEvent{}, false
}

func lastQuestion(events []SessionEvent) string {
	ev, ok := latest(events, EventStop)
	if !ok {
		return ""
	}
	text := strings.TrimSpace(ev.Text)
	paragraph := text[strings.LastIndex(text, "\n\n")+1:]
	paragraph = strings.TrimSpace(paragraph)
	if !strings.HasSuffix(paragraph, "?") {
		return ""
	}
	return paragraph
}

func cutLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, " \t\r\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n") + "…"
}

package domain

import "strings"

const (
	// CardActions is how many recent tool calls the card lists.
	CardActions = 3
	// CardWaitingLines is how many lines of a permission prompt the card shows.
	CardWaitingLines = 3
)

// SessionCard is what you need to get your bearings on a session: the task,
// its PRs, the last few tool calls (newest first) and what the agent waits
// on. Waiting is empty when it waits on nothing.
type SessionCard struct {
	Title   string
	Ref     string
	PRs     []PullRequest
	Actions []string
	Waiting string
}

// BuildSessionCard derives the card from the task, the session's worktrees
// and its stored events, oldest first. Events of other sessions are ignored.
func BuildSessionCard(task Task, session Session, worktrees []Worktree, events []SessionEvent) SessionCard {
	card := SessionCard{Title: NameFor(task, nil), Ref: task.Ref, PRs: pullRequests(worktrees)}
	var own []SessionEvent
	for _, ev := range events {
		if ev.SessionID == session.ID {
			own = append(own, ev)
		}
	}
	card.Actions = lastActions(own)
	card.Waiting = waitingOn(session.State, own)
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
		label := ev.Tool
		if ev.Detail != "" {
			label += ": " + ev.Detail
		}
		out = append(out, label)
	}
	return out
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

// lastQuestion is the final paragraph of the last assistant message when it
// ends in a question mark.
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

package domain

import (
	"fmt"
	"time"
)

type ReviewDecision string

const (
	ReviewNone             ReviewDecision = ""
	ReviewApproved         ReviewDecision = "APPROVED"
	ReviewChangesRequested ReviewDecision = "CHANGES_REQUESTED"
	ReviewRequired         ReviewDecision = "REVIEW_REQUIRED"
)

type Mergeable string

const (
	MergeUnknown     Mergeable = ""
	MergeClean       Mergeable = "MERGEABLE"
	MergeConflicting Mergeable = "CONFLICTING"
)

type FailingCheck struct {
	Name string
	URL  string
}

type PRComment struct {
	Bot bool
	At  time.Time
}

func BotCommentsSince(comments []PRComment, since time.Time) int {
	n := 0
	for _, c := range comments {
		if c.Bot && c.At.After(since) {
			n++
		}
	}
	return n
}

func (p PullRequest) Blockers() []string {
	if p.State != PROpen {
		return nil
	}
	var out []string
	switch p.Checks {
	case CheckFailing:
		out = append(out, "checks failing")
	case CheckPending:
		out = append(out, "checks pending")
	}
	switch p.ReviewDecision {
	case ReviewChangesRequested:
		out = append(out, "changes requested")
	case ReviewRequired:
		out = append(out, "review required")
	}
	if p.Mergeable == MergeConflicting {
		out = append(out, "merge conflicts")
	}
	switch p.UnresolvedThreads {
	case 0:
	case 1:
		out = append(out, "1 unresolved thread")
	default:
		out = append(out, fmt.Sprintf("%d unresolved threads", p.UnresolvedThreads))
	}
	return out
}

func (p PullRequest) ReadyToMerge() bool {
	return p.State == PROpen && len(p.Blockers()) == 0
}

const (
	fastPollDivisor = 4
	maxBackoffScale = 10
)

func NextPRPoll(base time.Duration, checksRunning bool, failures int) time.Duration {
	if failures > 0 {
		wait := base
		for range failures {
			wait *= 2
			if wait >= base*maxBackoffScale {
				return base * maxBackoffScale
			}
		}
		return wait
	}
	if checksRunning {
		return base / fastPollDivisor
	}
	return base
}

package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestPRBoardBlockers(t *testing.T) {
	ready := PullRequest{
		State: PROpen, Checks: CheckPassing,
		ReviewDecision: ReviewApproved, Mergeable: MergeClean,
	}
	tests := []struct {
		name string
		edit func(p *PullRequest)
		want []string
	}{
		{"ready", func(*PullRequest) {}, nil},
		{"no checks and no review policy", func(p *PullRequest) {
			p.Checks, p.ReviewDecision = CheckNone, ReviewNone
		}, nil},
		{"failing checks", func(p *PullRequest) { p.Checks = CheckFailing }, []string{"checks failing"}},
		{"pending checks", func(p *PullRequest) { p.Checks = CheckPending }, []string{"checks pending"}},
		{"changes requested", func(p *PullRequest) { p.ReviewDecision = ReviewChangesRequested }, []string{"changes requested"}},
		{"review required", func(p *PullRequest) { p.ReviewDecision = ReviewRequired }, []string{"review required"}},
		{"conflicts", func(p *PullRequest) { p.Mergeable = MergeConflicting }, []string{"merge conflicts"}},
		{"unknown mergeability is not a blocker", func(p *PullRequest) { p.Mergeable = MergeUnknown }, nil},
		{"one thread", func(p *PullRequest) { p.UnresolvedThreads = 1 }, []string{"1 unresolved thread"}},
		{"threads", func(p *PullRequest) { p.UnresolvedThreads = 3 }, []string{"3 unresolved threads"}},
		{"everything at once", func(p *PullRequest) {
			p.Checks, p.ReviewDecision, p.Mergeable, p.UnresolvedThreads = CheckFailing, ReviewChangesRequested, MergeConflicting, 2
		}, []string{"checks failing", "changes requested", "merge conflicts", "2 unresolved threads"}},
		{"merged is never blocked", func(p *PullRequest) {
			p.State, p.Checks, p.UnresolvedThreads = PRMerged, CheckFailing, 4
		}, nil},
		{"closed is never blocked", func(p *PullRequest) { p.State, p.Mergeable = PRClosed, MergeConflicting }, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := ready
			tc.edit(&p)
			if got := p.Blockers(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Blockers = %q, want %q", got, tc.want)
			}
			wantReady := p.State == PROpen && len(tc.want) == 0
			if p.ReadyToMerge() != wantReady {
				t.Errorf("ReadyToMerge = %v, want %v", p.ReadyToMerge(), wantReady)
			}
		})
	}
}

func TestPRBoardBotCommentsSinceCountsOnlyBotCommentsAfterThePush(t *testing.T) {
	push := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	comments := []PRComment{
		{Bot: true, At: push.Add(-time.Minute)},
		{Bot: true, At: push},
		{Bot: true, At: push.Add(time.Second)},
		{Bot: false, At: push.Add(time.Hour)},
		{Bot: true, At: push.Add(time.Hour)},
	}
	if got := BotCommentsSince(comments, push); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if got := BotCommentsSince(comments, time.Time{}); got != 4 {
		t.Errorf("with no known push got %d, want every bot comment (4)", got)
	}
}

func TestPRBoardNextPoll(t *testing.T) {
	base := 60 * time.Second
	tests := []struct {
		name     string
		pending  bool
		failures int
		want     time.Duration
	}{
		{"idle", false, 0, base},
		{"checks running poll faster", true, 0, 15 * time.Second},
		{"one failure doubles", false, 1, 2 * base},
		{"failures keep doubling", false, 3, 8 * base},
		{"backoff is capped", false, 20, 10 * base},
		{"backoff beats fast polling", true, 2, 4 * base},
	}
	for _, tc := range tests {
		if got := NextPRPoll(base, tc.pending, tc.failures); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

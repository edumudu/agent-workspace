package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestPRBoardBlockers(t *testing.T) {
	ready := domain.PullRequest{
		State: domain.PROpen, Checks: domain.CheckPassing,
		ReviewDecision: domain.ReviewApproved, Mergeable: domain.MergeClean,
	}
	tests := []struct {
		name string
		edit func(p *domain.PullRequest)
		want []string
	}{
		{"ready", func(*domain.PullRequest) {}, nil},
		{"no checks and no review policy", func(p *domain.PullRequest) {
			p.Checks, p.ReviewDecision = domain.CheckNone, domain.ReviewNone
		}, nil},
		{"failing checks", func(p *domain.PullRequest) { p.Checks = domain.CheckFailing }, []string{"checks failing"}},
		{"pending checks", func(p *domain.PullRequest) { p.Checks = domain.CheckPending }, []string{"checks pending"}},
		{"changes requested", func(p *domain.PullRequest) { p.ReviewDecision = domain.ReviewChangesRequested }, []string{"changes requested"}},
		{"review required", func(p *domain.PullRequest) { p.ReviewDecision = domain.ReviewRequired }, []string{"review required"}},
		{"conflicts", func(p *domain.PullRequest) { p.Mergeable = domain.MergeConflicting }, []string{"merge conflicts"}},
		{"unknown mergeability is not a blocker", func(p *domain.PullRequest) { p.Mergeable = domain.MergeUnknown }, nil},
		{"one thread", func(p *domain.PullRequest) { p.UnresolvedThreads = 1 }, []string{"1 unresolved thread"}},
		{"threads", func(p *domain.PullRequest) { p.UnresolvedThreads = 3 }, []string{"3 unresolved threads"}},
		{"everything at once", func(p *domain.PullRequest) {
			p.Checks, p.ReviewDecision, p.Mergeable, p.UnresolvedThreads = domain.CheckFailing, domain.ReviewChangesRequested, domain.MergeConflicting, 2
		}, []string{"checks failing", "changes requested", "merge conflicts", "2 unresolved threads"}},
		{"merged is never blocked", func(p *domain.PullRequest) {
			p.State, p.Checks, p.UnresolvedThreads = domain.PRMerged, domain.CheckFailing, 4
		}, nil},
		{"closed is never blocked", func(p *domain.PullRequest) { p.State, p.Mergeable = domain.PRClosed, domain.MergeConflicting }, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := ready
			tc.edit(&p)
			if got := p.Blockers(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Blockers = %q, want %q", got, tc.want)
			}
			wantReady := p.State == domain.PROpen && len(tc.want) == 0
			if p.ReadyToMerge() != wantReady {
				t.Errorf("ReadyToMerge = %v, want %v", p.ReadyToMerge(), wantReady)
			}
		})
	}
}

func TestBotCommentsSinceCountsOnlyBotCommentsAfterThePush(t *testing.T) {
	push := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	comments := []domain.PRComment{
		{Bot: true, At: push.Add(-time.Minute)},
		{Bot: true, At: push},
		{Bot: true, At: push.Add(time.Second)},
		{Bot: false, At: push.Add(time.Hour)},
		{Bot: true, At: push.Add(time.Hour)},
	}
	if got := domain.BotCommentsSince(comments, push); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if got := domain.BotCommentsSince(comments, time.Time{}); got != 4 {
		t.Errorf("with no known push got %d, want every bot comment (4)", got)
	}
}

func TestNextPRPoll(t *testing.T) {
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
		if got := domain.NextPRPoll(base, tc.pending, tc.failures); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

package tui_test

import (
	"strings"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestPRBoardCardListsBlockersFailingChecksAndBotComments(t *testing.T) {
	st := cardFixture()
	for i := range st.Worktrees {
		switch st.Worktrees[i].ID {
		case "w05-0":
			st.Worktrees[i].PR = &domain.PullRequest{
				Number: 3604, Title: "session 5 change", State: domain.PROpen,
				Checks: domain.CheckFailing, ReviewDecision: domain.ReviewChangesRequested, UnresolvedThreads: 2, BotComments: 3,
				Failing: []domain.FailingCheck{{Name: "unit tests", URL: "https://example.com/run/1"}},
			}
		case "w03-0":
			st.Worktrees[i].PR = &domain.PullRequest{
				Number: 3602, State: domain.PROpen, Checks: domain.CheckPassing,
				ReviewDecision: domain.ReviewApproved, Mergeable: domain.MergeClean,
			}
		}
	}
	out := screen(selectSession(t, newModel(&st, nil), "s05"))
	card := out[strings.Index(out, "CARD"):]
	for _, want := range []string{"#3604 blocked", "checks failing", "changes requested", "2 unresolved threads", "✗ unit tests", "3 bot comments since push"} {
		if !strings.Contains(card, want) {
			t.Errorf("card missing %q:\n%s", want, card)
		}
	}

	out = screen(selectSession(t, newModel(&st, nil), "s03"))
	card = out[strings.Index(out, "CARD"):]
	if !strings.Contains(card, "#3602 ready to merge") {
		t.Errorf("card missing ready state:\n%s", card)
	}
}

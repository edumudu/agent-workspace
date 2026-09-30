package tui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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

func failingCheckModel(t *testing.T, url string) string {
	t.Helper()
	st := cardFixture()
	for i := range st.Worktrees {
		if st.Worktrees[i].ID == "w05-0" {
			st.Worktrees[i].PR = &domain.PullRequest{
				Number: 3604, State: domain.PROpen, Checks: domain.CheckFailing,
				Failing: []domain.FailingCheck{{Name: "unit tests", URL: url}},
			}
		}
	}
	return selectSession(t, newModel(&st, nil), "s05").View().Content
}

func TestPRBoardCardLinksFailingCheckToItsRun(t *testing.T) {
	raw := failingCheckModel(t, "https://example.com/run/1")
	if !strings.Contains(raw, "\x1b]8;;https://example.com/run/1") {
		t.Errorf("no OSC 8 link to the run in %q", raw)
	}
	if !strings.Contains(ansi.Strip(raw), "✗ unit tests") {
		t.Errorf("link swallowed the check name:\n%s", ansi.Strip(raw))
	}
}

func TestPRBoardCardNeverLinksAnUnsafeURL(t *testing.T) {
	for _, url := range []string{"", "javascript:alert(1)", "https://x.example/\x1b]8;;https://evil.example", "https://x.example/a\x07b", "ftp://x.example/run"} {
		raw := failingCheckModel(t, url)
		if strings.Contains(raw, "\x1b]8;;") {
			t.Errorf("url %q was linked: %q", url, raw)
		}
		if !strings.Contains(ansi.Strip(raw), "✗ unit tests") {
			t.Errorf("url %q: check name missing", url)
		}
	}
}

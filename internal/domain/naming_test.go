package domain_test

import (
	"testing"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestNameFor(t *testing.T) {
	pr := []domain.PullRequest{{Number: 42, Title: "Add login"}}
	cases := []struct {
		name string
		task domain.Task
		prs  []domain.PullRequest
		want string
	}{
		{"pinned beats everything", domain.Task{PinnedName: "mine", IssueTitle: "Issue", Text: "text"}, pr, "mine"},
		{"pr title beats issue", domain.Task{IssueTitle: "Issue", Text: "text"}, pr, "Add login"},
		{"issue beats text", domain.Task{IssueTitle: "Issue", Text: "text"}, nil, "Issue"},
		{"text last", domain.Task{Text: "fix the thing"}, nil, "fix the thing"},
		{"blank pr title skipped", domain.Task{Text: "text"}, []domain.PullRequest{{Number: 1, Title: "  "}, {Number: 2, Title: "Second"}}, "Second"},
		{"blank pinned ignored", domain.Task{PinnedName: " ", Text: "text"}, nil, "text"},
		{"blank issue ignored", domain.Task{IssueTitle: "\t", Text: "text"}, nil, "text"},
		{"trims", domain.Task{Text: "  spaced  "}, nil, "spaced"},
		{"nothing", domain.Task{}, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.NameFor(c.task, c.prs); got != c.want {
				t.Errorf("NameFor = %q, want %q", got, c.want)
			}
		})
	}
}

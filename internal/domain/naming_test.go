package domain

import (
	"testing"
)

func TestNameFor(t *testing.T) {
	pr := []PullRequest{{Number: 42, Title: "Add login"}}
	cases := []struct {
		name string
		task Task
		prs  []PullRequest
		want string
	}{
		{"pinned beats everything", Task{PinnedName: "mine", IssueTitle: "Issue", Text: "text"}, pr, "mine"},
		{"pr title beats issue", Task{IssueTitle: "Issue", Text: "text"}, pr, "Add login"},
		{"issue beats text", Task{IssueTitle: "Issue", Text: "text"}, nil, "Issue"},
		{"text last", Task{Text: "fix the thing"}, nil, "fix the thing"},
		{"blank pr title skipped", Task{Text: "text"}, []PullRequest{{Number: 1, Title: "  "}, {Number: 2, Title: "Second"}}, "Second"},
		{"blank pinned ignored", Task{PinnedName: " ", Text: "text"}, nil, "text"},
		{"blank issue ignored", Task{IssueTitle: "\t", Text: "text"}, nil, "text"},
		{"trims", Task{Text: "  spaced  "}, nil, "spaced"},
		{"nothing", Task{}, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NameFor(c.task, c.prs); got != c.want {
				t.Errorf("NameFor = %q, want %q", got, c.want)
			}
		})
	}
}

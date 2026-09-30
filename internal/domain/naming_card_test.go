package domain

import "testing"

func TestNamingCardCarriesTheFullSessionName(t *testing.T) {
	prWorktree := []Worktree{{ID: "w1", PR: &PullRequest{Number: 7, Title: "Retry uploads with backoff"}}}
	cases := []struct {
		name      string
		task      Task
		worktrees []Worktree
		want      string
	}{
		{"the issue title before a PR exists", cardTask, nil, "Retry failed uploads"},
		{"the PR title once a worktree has one", cardTask, prWorktree, "Retry uploads with backoff"},
		{"a pin over the PR", Task{ID: "t1", IssueTitle: "x", PinnedName: "uploads"}, prWorktree, "uploads"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := BuildSessionCard(c.task, Session{ID: "s1"}, c.worktrees, nil).Name; got != c.want {
				t.Errorf("card name = %q, want %q", got, c.want)
			}
		})
	}
}

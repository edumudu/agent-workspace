package domain

import "testing"

func TestAgentTitleNamesTheHarnessModelCwdAndEachWorktreeWithItsPR(t *testing.T) {
	s := Session{Harness: HarnessClaude, Model: "opus-5.5", Effort: "high", State: StateRunning, WorktreeIDs: []string{"w1", "w2", "w3"}}
	wts := []Worktree{
		{ID: "w1", Repo: "/src/platform/api", SubtaskSlug: "org-scope", PR: &PullRequest{Number: 3611, State: PROpen, Checks: CheckPending}},
		{ID: "w2", Repo: "/src/platform/web", Branch: "share-token", PR: &PullRequest{Number: 5731, State: PROpen, Checks: CheckPassing}},
		{ID: "w3", Repo: "/src/platform/api", SubtaskSlug: "legacy-removal"},
	}
	got := AgentTitle(s, wts, "/src/platform")
	want := "◐ claude · opus-5.5 · high · cwd platform │ api:org-scope #3611 ◐ │ web:share-token #5731 ✓ │ api:legacy-removal no PR"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAgentTitleMarksStateAndPROutcomes(t *testing.T) {
	cases := []struct {
		name string
		s    Session
		pr   *PullRequest
		want string
	}{
		{"waiting", Session{Harness: HarnessCodex, State: StateWaiting}, nil, "✳ codex"},
		{"permission", Session{Harness: HarnessClaude, State: StatePermission}, nil, "✳ claude"},
		{"done", Session{Harness: HarnessClaude, State: StateDone}, nil, "● claude"},
		{"idle", Session{Harness: HarnessClaude, State: StateIdle}, nil, "○ claude"},
		{"failing checks", Session{Harness: HarnessClaude, State: StateIdle, WorktreeIDs: []string{"w"}},
			&PullRequest{Number: 7, State: PROpen, Checks: CheckFailing}, "○ claude │ api:x #7 ✗"},
		{"merged", Session{Harness: HarnessClaude, State: StateIdle, WorktreeIDs: []string{"w"}},
			&PullRequest{Number: 7, State: PRMerged}, "○ claude │ api:x #7 merged"},
		{"open without checks", Session{Harness: HarnessClaude, State: StateIdle, WorktreeIDs: []string{"w"}},
			&PullRequest{Number: 7, State: PROpen}, "○ claude │ api:x #7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var wts []Worktree
			if len(c.s.WorktreeIDs) > 0 {
				wts = []Worktree{{ID: "w", Repo: "/src/api", SubtaskSlug: "x", PR: c.pr}}
			}
			if got := AgentTitle(c.s, wts, ""); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestAgentTitleSkipsWorktreesItDoesNotOwn(t *testing.T) {
	s := Session{Harness: HarnessClaude, State: StateIdle, WorktreeIDs: []string{"mine"}}
	wts := []Worktree{{ID: "other", Repo: "/src/web", SubtaskSlug: "y"}, {ID: "mine", Repo: "/src/api", SubtaskSlug: "x"}}
	if got := AgentTitle(s, wts, ""); got != "○ claude │ api:x no PR" {
		t.Fatalf("got %q", got)
	}
}

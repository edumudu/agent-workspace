package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestWorktreeDetectIsWorktreeAdd(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{"git worktree add ../api-feat -b feat", true},
		{"cd /w/api && git worktree add /w/api-x", true},
		{"git -C /w/web worktree add ../web-x main", true},
		{"git -c core.x=1 --no-pager worktree add x", true},
		{"/usr/bin/git worktree add x", true},
		{"make; git worktree  add x | tee log", true},
		{"git worktree list", false},
		{"git worktree remove x", false},
		{"echo git worktree add", false},
		{"git status", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsWorktreeAdd(c.command); got != c.want {
			t.Errorf("IsWorktreeAdd(%q) = %v, want %v", c.command, got, c.want)
		}
	}
}

func TestWorktreeDetectSubagentParent(t *testing.T) {
	cases := []struct {
		path   string
		parent string
		ok     bool
	}{
		{"/w/api/.claude/worktrees/agent-a1b2", "/w/api", true},
		{"/w/api/.claude/worktrees/agent-a1b2/", "/w/api", true},
		{"/w/api/.claude/worktrees/feature", "", false},
		{"/w/api/worktrees/agent-a1", "", false},
		{"/w/api-feat", "", false},
	}
	for _, c := range cases {
		parent, ok := SubagentParent(c.path)
		if parent != c.parent || ok != c.ok {
			t.Errorf("SubagentParent(%q) = %q, %v; want %q, %v", c.path, parent, ok, c.parent, c.ok)
		}
	}
}

func TestWorktreeDetectAttribute(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	hints := []SessionHint{{ID: "s1", Cwd: "/w/api"}, {ID: "s2", Cwd: "/w/web-x/src"}}
	recent := now.Add(-5 * time.Second)
	stale := now.Add(-ClaimWindow - time.Second)
	cases := []struct {
		name   string
		wt     ListedWorktree
		claims []WorktreeClaim
		want   string
	}{
		{"subagent worktree goes to the parent session", ListedWorktree{Path: "/w/api/.claude/worktrees/agent-1"}, nil, "s1"},
		{"session working inside the worktree", ListedWorktree{Path: "/w/web-x"}, nil, "s2"},
		{"session cwd equal to the worktree", ListedWorktree{Path: "/w/api"}, nil, "s1"},
		{"a prefix that is not a parent dir does not match", ListedWorktree{Path: "/w/web"}, nil, ""},
		{"single recent claim", ListedWorktree{Path: "/w/api-feat"}, []WorktreeClaim{{SessionID: "s1", Command: "git worktree add ../x", At: recent}}, "s1"},
		{"stale claim", ListedWorktree{Path: "/w/api-feat"}, []WorktreeClaim{{SessionID: "s1", Command: "git worktree add ../x", At: stale}}, ""},
		{"several claims, one names the path", ListedWorktree{Path: "/w/api-feat", Branch: "feat"}, []WorktreeClaim{
			{SessionID: "s1", Command: "git worktree add ../other", At: recent},
			{SessionID: "s2", Command: "git worktree add ../api-feat", At: recent},
		}, "s2"},
		{"several claims, one names the branch", ListedWorktree{Path: "/w/api-z", Branch: "fix-login"}, []WorktreeClaim{
			{SessionID: "s1", Command: "git worktree add -b fix-login ../z", At: recent},
			{SessionID: "s2", Command: "git worktree add ../q", At: recent},
		}, "s1"},
		{"several claims, none match", ListedWorktree{Path: "/w/api-z", Branch: "b"}, []WorktreeClaim{
			{SessionID: "s1", Command: "git worktree add ../x", At: recent},
			{SessionID: "s2", Command: "git worktree add ../y", At: recent},
		}, ""},
		{"two claims by the same session", ListedWorktree{Path: "/w/api-z"}, []WorktreeClaim{
			{SessionID: "s1", Command: "git worktree add ../x", At: recent},
			{SessionID: "s1", Command: "git worktree add ../y", At: recent},
		}, "s1"},
		{"nothing matches", ListedWorktree{Path: "/tmp/hand"}, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AttributeWorktree(c.wt, hints, c.claims, now); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestWorktreeDetectReconcile(t *testing.T) {
	known := []Worktree{
		{ID: "/w/api-a", Repo: "/w/api", Path: "/w/api-a", Branch: "a", SessionID: "s1", PR: &PullRequest{Number: 3}},
		{ID: "/w/api-gone", Repo: "/w/api", Path: "/w/api-gone", Branch: "g", SessionID: "s2"},
		{ID: "/w/web-b", Repo: "/w/web", Path: "/w/web-b", Branch: "b"},
	}
	listing := RepoListing{Main: "/w/api", Worktrees: []ListedWorktree{
		{Path: "/w/api-a", Branch: "a2"},
		{Path: "/w/api-new", Branch: "n"},
	}}
	attribute := func(wt ListedWorktree) string { return "s9" }
	changed, removed := ReconcileWorktrees(known, listing, attribute)
	wantChanged := []Worktree{
		{ID: "/w/api-a", Repo: "/w/api", Path: "/w/api-a", Branch: "a2", SessionID: "s1", PR: &PullRequest{Number: 3}},
		{ID: "/w/api-new", Repo: "/w/api", Path: "/w/api-new", Branch: "n", SessionID: "s9"},
	}
	if !reflect.DeepEqual(changed, wantChanged) {
		t.Errorf("changed = %+v, want %+v", changed, wantChanged)
	}
	if !reflect.DeepEqual(removed, []string{"/w/api-gone"}) {
		t.Errorf("removed = %v", removed)
	}
}

func TestWorktreeDetectReconcileUnchangedListing(t *testing.T) {
	known := []Worktree{{ID: "/w/api-a", Repo: "/w/api", Path: "/w/api-a", Branch: "a", SessionID: "s1"}}
	listing := RepoListing{Main: "/w/api", Worktrees: []ListedWorktree{{Path: "/w/api-a", Branch: "a"}}}
	changed, removed := ReconcileWorktrees(known, listing, func(ListedWorktree) string { return "x" })
	if len(changed) != 0 || len(removed) != 0 {
		t.Errorf("changed %v, removed %v", changed, removed)
	}
}

func TestWorktreeDetectReclaimSeenBeforeItsClaim(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-5 * time.Second)
	known := []Worktree{
		{ID: "/w/api", Repo: "/w/api", Path: "/w/api", Branch: "main"},
		{ID: "/w/api-a", Repo: "/w/api", Path: "/w/api-a", Branch: "feat-a"},
		{ID: "/w/api-b", Repo: "/w/api", Path: "/w/api-b", Branch: "feat-b"},
		{ID: "/w/api-c", Repo: "/w/api", Path: "/w/api-c", Branch: "feat-c", SessionID: "s2"},
		{ID: "/w/api-old", Repo: "/w/api", Path: "/w/api-old", Branch: "old"},
		{ID: "/w/api-z", Repo: "/w/api", Path: "/w/api-z", Branch: "z"},
		{ID: "/w/api-hand", Repo: "/w/api", Path: "/w/api-hand", Branch: "old"},
	}
	claims := []WorktreeClaim{
		{SessionID: "s1", Cwd: "/w/api", Command: "git worktree add -q -b feat-a ../api-a", At: recent},
		{SessionID: "s1", Cwd: "/w/api", Command: "git worktree add ../api-b", At: recent},
		{SessionID: "s1", Cwd: "/w/api", Command: "git worktree add ../api-c", At: recent},
		{SessionID: "s1", Cwd: "/w/api", Command: "git worktree add ../api-old", At: now.Add(-ClaimWindow - time.Second)},
		{SessionID: "s1", Cwd: "/w/api", Command: "git worktree add ../api-z", At: recent},
		{SessionID: "s3", Cwd: "/w/api", Command: "git worktree add -b z ../elsewhere", At: recent},
		{SessionID: "s1", Cwd: "/w/api", Command: "git worktree add /var/folders/x/api-hand-2", At: recent},
	}
	got := ReclaimWorktrees(known, claims, now)
	want := []Worktree{
		{ID: "/w/api-a", Repo: "/w/api", Path: "/w/api-a", Branch: "feat-a", SessionID: "s1"},
		{ID: "/w/api-b", Repo: "/w/api", Path: "/w/api-b", Branch: "feat-b", SessionID: "s1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestWorktreeDetectReclaimOnlyFromClaimsInTheSameRepo(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	known := []Worktree{
		{ID: "/w/api-s", Repo: "/w/api", Path: "/w/api-s", Branch: "s", SessionID: "s1"},
		{ID: "/w/web-s", Repo: "/w/web", Path: "/w/web-s", Branch: "t", SessionID: "s2"},
		{ID: "/w/api-x", Repo: "/w/api", Path: "/w/api-x", Branch: "feat"},
		{ID: "/w/web-x", Repo: "/w/web", Path: "/w/web-x", Branch: "feat"},
		{ID: "/w/lib-x", Repo: "/w/lib", Path: "/w/lib-x", Branch: "feat"},
	}
	claims := []WorktreeClaim{
		{SessionID: "s1", Cwd: "/w/api-s/src", Command: "git worktree add -b feat ../api-x", At: now},
		{SessionID: "s2", Cwd: "/w/web", Command: "git worktree add -b feat ../web-x", At: now},
		{SessionID: "s3", Command: "git worktree add -b feat ../lib-x", At: now},
	}
	got := ReclaimWorktrees(known, claims, now)
	want := []Worktree{
		{ID: "/w/api-x", Repo: "/w/api", Path: "/w/api-x", Branch: "feat", SessionID: "s1"},
		{ID: "/w/web-x", Repo: "/w/web", Path: "/w/web-x", Branch: "feat", SessionID: "s2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestWorktreeDetectRollupChecks(t *testing.T) {
	cases := []struct {
		name   string
		states []CheckState
		want   CheckState
	}{
		{"no checks", nil, CheckNone},
		{"all passing", []CheckState{CheckPassing, CheckPassing}, CheckPassing},
		{"one pending", []CheckState{CheckPassing, CheckPending}, CheckPending},
		{"failure beats pending", []CheckState{CheckPending, CheckFailing, CheckPassing}, CheckFailing},
		{"skipped ones are ignored", []CheckState{CheckNone, CheckPassing}, CheckPassing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RollupChecks(c.states); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestWorktreeDetectPRForBranch(t *testing.T) {
	prs := []PullRequest{
		{Number: 1, Head: "feat", State: PRMerged},
		{Number: 4, Head: "feat", State: PROpen},
		{Number: 2, Head: "feat", State: PRClosed},
		{Number: 7, Head: "other", State: PROpen},
		{Number: 9, Head: "old", State: PRClosed},
		{Number: 8, Head: "old", State: PRMerged},
	}
	cases := []struct {
		branch string
		want   int
	}{
		{"feat", 4},
		{"old", 9},
		{"none", 0},
		{"", 0},
	}
	for _, c := range cases {
		got := PRForBranch(prs, c.branch)
		n := 0
		if got != nil {
			n = got.Number
		}
		if n != c.want {
			t.Errorf("PRForBranch(%q) = #%d, want #%d", c.branch, n, c.want)
		}
	}
}

func TestWorktreeDetectSessionAttachAndDetach(t *testing.T) {
	s := Session{ID: "s", WorktreeIDs: []string{"a"}}
	s = s.AttachWorktree("b").AttachWorktree("a")
	if !reflect.DeepEqual(s.WorktreeIDs, []string{"a", "b"}) {
		t.Fatalf("after attach: %v", s.WorktreeIDs)
	}
	orig := s
	s = s.DetachWorktree("a").DetachWorktree("zz")
	if !reflect.DeepEqual(s.WorktreeIDs, []string{"b"}) {
		t.Fatalf("after detach: %v", s.WorktreeIDs)
	}
	if !reflect.DeepEqual(orig.WorktreeIDs, []string{"a", "b"}) {
		t.Fatalf("detach modified its receiver's slice: %v", orig.WorktreeIDs)
	}
}

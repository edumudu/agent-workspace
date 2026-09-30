package app_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestWorktreeDetectScanListsEachRepoOnce(t *testing.T) {
	api := domain.RepoListing{Main: "/w/api", Worktrees: []domain.ListedWorktree{{Path: "/w/api-a", Branch: "a"}}}
	web := domain.RepoListing{Main: "/w/web"}
	lister := fakeLister{"/w/web": web, "/w/api": api, "/w/api-a/src": api}
	got := app.ScanWorktrees(context.Background(), lister, []string{"/w/web", "/w/api", "/w/api-a/src", "/w/broken"})
	want := []domain.RepoListing{api, web}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestWorktreeDetectRefreshPRsReturnsOnlyChanges(t *testing.T) {
	finder := &fakeFinder{calls: map[string]int{}, prs: map[string][]domain.PullRequest{
		"/w/api": {
			{Number: 5, Head: "a", State: domain.PROpen, Checks: domain.CheckPending},
			{Number: 6, Head: "b", State: domain.PROpen},
		},
	}}
	worktrees := []domain.Worktree{
		{ID: "/w/api-a", Repo: "/w/api", Branch: "a"},
		{ID: "/w/api-b", Repo: "/w/api", Branch: "b", PR: &domain.PullRequest{Number: 6, Head: "b", State: domain.PROpen}},
		{ID: "/w/api-c", Repo: "/w/api", Branch: "c", PR: &domain.PullRequest{Number: 1, Head: "c"}},
		{ID: "/w/web-x", Repo: "/w/web", Branch: "x", PR: &domain.PullRequest{Number: 2}},
	}
	got := app.RefreshPRs(context.Background(), finder, worktrees)
	want := []domain.Worktree{
		{ID: "/w/api-a", Repo: "/w/api", Branch: "a", PR: &domain.PullRequest{Number: 5, Head: "a", State: domain.PROpen, Checks: domain.CheckPending}},
		{ID: "/w/api-c", Repo: "/w/api", Branch: "c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if finder.calls["/w/api"] != 1 || finder.calls["/w/web"] != 1 {
		t.Errorf("calls = %v, want one per repo", finder.calls)
	}
}

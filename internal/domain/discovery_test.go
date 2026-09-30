package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestDiscoveryKindOfRoot(t *testing.T) {
	cases := []struct {
		name string
		git  GitMarker
		want WorkspaceKind
	}{
		{"git directory is a single repo", GitDir, WorkspaceSingle},
		{"no .git is an orchestration root", GitNone, WorkspaceOrchestration},
		{"git file is an orchestration root", GitFile, WorkspaceOrchestration},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := KindOfRoot(c.git); got != c.want {
				t.Errorf("KindOfRoot(%v) = %q, want %q", c.git, got, c.want)
			}
		})
	}
}

func TestDiscoveryReposIn(t *testing.T) {
	children := []Child{
		{Name: "web", Path: "/r/web", Git: GitDir},
		{Name: "api", Path: "/r/api", Git: GitDir},
		{Name: "api-wt", Path: "/r/api-wt", Git: GitFile},
		{Name: "docs", Path: "/r/docs", Git: GitNone},
		{Name: "linked", Path: "/r/linked", Git: GitDir},
	}
	want := []Repo{
		{Name: "api", Path: "/r/api"},
		{Name: "linked", Path: "/r/linked"},
		{Name: "web", Path: "/r/web"},
	}
	if got := ReposIn(children); !reflect.DeepEqual(got, want) {
		t.Errorf("ReposIn = %+v, want %+v", got, want)
	}
}

func TestDiscoveryReposInNone(t *testing.T) {
	if got := ReposIn([]Child{{Name: "x", Git: GitFile}}); len(got) != 0 {
		t.Errorf("ReposIn = %+v, want none", got)
	}
}

func TestDiscoverySingleRepoIsItsOwnRepo(t *testing.T) {
	got := SingleRepo("/home/me/api")
	want := []Repo{{Name: "api", Path: "/home/me/api"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SingleRepo = %+v, want %+v", got, want)
	}
}

func TestDiscoveryMergeStateKeepsWhatIsStillThere(t *testing.T) {
	old := []Repo{
		{Name: "api", Path: "/r/api", DefaultBranch: "main", Branch: "feat", ChangedFiles: 3},
		{Name: "gone", Path: "/r/gone", Branch: "x"},
	}
	found := []Repo{{Name: "api", Path: "/r/api"}, {Name: "new", Path: "/r/new"}}
	want := []Repo{
		{Name: "api", Path: "/r/api", DefaultBranch: "main", Branch: "feat", ChangedFiles: 3},
		{Name: "new", Path: "/r/new"},
	}
	if got := MergeRepoState(old, found); !reflect.DeepEqual(got, want) {
		t.Errorf("MergeRepoState = %+v, want %+v", got, want)
	}
}

func TestDiscoveryLastUsed(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ws := []Workspace{
		{Root: "/a", LastUsed: t0},
		{Root: "/b", LastUsed: t0.Add(time.Hour)},
		{Root: "/c"},
	}
	got, ok := LastUsedWorkspace(ws)
	if !ok || got.Root != "/b" {
		t.Errorf("LastUsedWorkspace = %v, %v; want /b", got.Root, ok)
	}
	if _, ok := LastUsedWorkspace(nil); ok {
		t.Error("LastUsedWorkspace(nil) reported a workspace")
	}
	if _, ok := LastUsedWorkspace([]Workspace{{Root: "/c"}}); ok {
		t.Error("a workspace that was never used counts as last used")
	}
}

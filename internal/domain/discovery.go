package domain

import (
	"path/filepath"
	"sort"
)

type GitMarker int

const (
	GitNone GitMarker = iota
	GitDir
	GitFile
)

type Child struct {
	Name string
	Path string
	Git  GitMarker
}

func KindOfRoot(rootGit GitMarker) WorkspaceKind {
	if rootGit == GitDir {
		return WorkspaceSingle
	}
	return WorkspaceOrchestration
}

func ReposIn(children []Child) []Repo {
	var repos []Repo
	for _, c := range children {
		if c.Git == GitDir {
			repos = append(repos, Repo{Name: c.Name, Path: c.Path})
		}
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
	return repos
}

func SingleRepo(root string) []Repo {
	return []Repo{{Name: filepath.Base(root), Path: root}}
}

func MergeRepoState(known, found []Repo) []Repo {
	byPath := make(map[string]Repo, len(known))
	for _, r := range known {
		byPath[r.Path] = r
	}
	out := make([]Repo, len(found))
	for i, r := range found {
		if k, ok := byPath[r.Path]; ok {
			out[i] = k
			out[i].Name = r.Name
		} else {
			out[i] = r
		}
	}
	return out
}

func LastUsedWorkspace(all []Workspace) (Workspace, bool) {
	var best Workspace
	found := false
	for _, w := range all {
		if w.LastUsed.IsZero() {
			continue
		}
		if !found || w.LastUsed.After(best.LastUsed) {
			best, found = w, true
		}
	}
	return best, found
}

package app

import (
	"context"
	"reflect"
	"sort"
	"sync"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// WorktreeLister lists the worktrees of the repo that dir belongs to. dir may
// be the main checkout, any worktree, or a directory inside one.
type WorktreeLister interface {
	ListWorktrees(ctx context.Context, dir string) (domain.RepoListing, error)
}

// PRFinder lists a repo's recent PRs with Head, State and Checks set.
type PRFinder interface {
	PRs(ctx context.Context, repo string) ([]domain.PullRequest, error)
}

// ScanWorktrees lists every repo reachable from dirs once, sorted by main
// checkout. A dir git cannot read is skipped, so its repo's worktrees are
// never taken as removed.
func ScanWorktrees(ctx context.Context, lister WorktreeLister, dirs []string) []domain.RepoListing {
	listings := make([]*domain.RepoListing, len(dirs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, refreshParallelism)
	for i := range dirs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if l, err := lister.ListWorktrees(ctx, dirs[i]); err == nil {
				listings[i] = &l
			}
		}()
	}
	wg.Wait()
	byMain := map[string]domain.RepoListing{}
	for _, l := range listings {
		if l != nil {
			byMain[l.Main] = *l
		}
	}
	out := make([]domain.RepoListing, 0, len(byMain))
	for _, l := range byMain {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Main < out[j].Main })
	return out
}

// RefreshPRs asks for each repo's PRs once and returns the worktrees whose
// PR changed. A repo the finder fails on keeps its PRs.
func RefreshPRs(ctx context.Context, finder PRFinder, worktrees []domain.Worktree) []domain.Worktree {
	byRepo := map[string][]domain.Worktree{}
	var repos []string
	for _, w := range worktrees {
		if _, ok := byRepo[w.Repo]; !ok {
			repos = append(repos, w.Repo)
		}
		byRepo[w.Repo] = append(byRepo[w.Repo], w)
	}
	var changed []domain.Worktree
	for _, repo := range repos {
		prs, err := finder.PRs(ctx, repo)
		if err != nil {
			continue
		}
		for _, w := range byRepo[repo] {
			pr := domain.PRForBranch(prs, w.Branch)
			if !reflect.DeepEqual(pr, w.PR) {
				w.PR = pr
				changed = append(changed, w)
			}
		}
	}
	return changed
}

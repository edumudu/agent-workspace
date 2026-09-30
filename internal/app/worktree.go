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

// PRFinder lists the recent PRs of every repo in one request. A repo it
// cannot resolve is left out of the result.
type PRFinder interface {
	PRs(ctx context.Context, repos []string) (map[string][]domain.PullRequest, error)
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

// PRRefresh is Changed, the worktrees whose PR changed, and whether any open
// PR has checks running after the refresh.
type PRRefresh struct {
	Changed       []domain.Worktree
	ChecksRunning bool
}

// RefreshPRs asks for every repo's PRs in one request. On error nothing
// changes, and a repo missing from the answer keeps its PRs.
func RefreshPRs(ctx context.Context, finder PRFinder, worktrees []domain.Worktree) (PRRefresh, error) {
	var repos []string
	seen := map[string]bool{}
	for _, w := range worktrees {
		if !seen[w.Repo] {
			seen[w.Repo] = true
			repos = append(repos, w.Repo)
		}
	}
	if len(repos) == 0 {
		return PRRefresh{}, nil
	}
	found, err := finder.PRs(ctx, repos)
	if err != nil {
		return PRRefresh{}, err
	}
	var out PRRefresh
	for _, w := range worktrees {
		prs, ok := found[w.Repo]
		if !ok {
			if running(w.PR) {
				out.ChecksRunning = true
			}
			continue
		}
		pr := domain.PRForBranch(prs, w.Branch)
		if running(pr) {
			out.ChecksRunning = true
		}
		if !reflect.DeepEqual(pr, w.PR) {
			w.PR = pr
			out.Changed = append(out.Changed, w)
		}
	}
	return out, nil
}

func running(pr *domain.PullRequest) bool {
	return pr != nil && pr.State == domain.PROpen && pr.Checks == domain.CheckPending
}

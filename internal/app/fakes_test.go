package app_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

type fakeHost struct {
	app.TerminalHost
	panes     []app.PaneInfo
	listErr   error
	created   []app.PaneSpec
	createErr error
	killed    []app.PaneID
	killErr   error
	log       *[]string
}

func (f *fakeHost) List(context.Context) ([]app.PaneInfo, error) { return f.panes, f.listErr }

func (f *fakeHost) Create(_ context.Context, spec app.PaneSpec) (app.PaneID, error) {
	if f.log != nil {
		*f.log = append(*f.log, "create "+spec.Dir)
	}
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, spec)
	return "%9", nil
}

func (f *fakeHost) Kill(_ context.Context, pane app.PaneID) error {
	if f.killErr != nil {
		return f.killErr
	}
	f.killed = append(f.killed, pane)
	return nil
}

type addedWorktree struct{ repo, path, branch, base string }

type fakeWorktrees struct {
	added []addedWorktree
	err   error
	log   *[]string
}

// AddWorktree reports paths under /real, as git does once symlinks resolve.
func (f *fakeWorktrees) AddWorktree(_ context.Context, repo, path, branch, base string) (app.AddedWorktree, error) {
	if f.log != nil {
		*f.log = append(*f.log, "add "+path)
	}
	if f.err != nil {
		return app.AddedWorktree{}, f.err
	}
	f.added = append(f.added, addedWorktree{repo, path, branch, base})
	return app.AddedWorktree{Main: "/real" + repo, Path: "/real" + path}, nil
}

type fakeHarness struct{}

func (fakeHarness) Harness() domain.Harness { return domain.HarnessCodex }

func (fakeHarness) Launch(req app.LaunchRequest) app.PaneSpec {
	return app.PaneSpec{Name: req.Name, Dir: req.Dir, Command: []string{"agent", req.Model, req.Effort, req.Prompt}}
}

type fakeFS struct {
	markers  map[string]domain.GitMarker
	children map[string][]domain.Child
}

func (f fakeFS) Marker(path string) (domain.GitMarker, error) {
	m, ok := f.markers[path]
	if !ok {
		return domain.GitNone, errors.New("no such directory")
	}
	return m, nil
}

func (f fakeFS) Children(path string) ([]domain.Child, error) { return f.children[path], nil }

type fakeGit map[string]app.RepoFacts

func (g fakeGit) Inspect(_ context.Context, path string) (app.RepoFacts, error) {
	f, ok := g[path]
	if !ok {
		return app.RepoFacts{}, errors.New("not a repo")
	}
	return f, nil
}

type fakeSetupWorld struct {
	recipe   *domain.Recipe
	main     string
	existing map[string]bool
	locks    map[string]domain.Lockfile
	free     []int64
	runErr   map[string]error
	ops      []string
	ticks    int
}

func (w *fakeSetupWorld) Load(string) (domain.Recipe, bool, error) {
	if w.recipe == nil {
		return domain.Recipe{}, false, nil
	}
	return *w.recipe, true, nil
}

func (w *fakeSetupWorld) MainCheckout(context.Context, string) (string, error) { return w.main, nil }

func (w *fakeSetupWorld) Exists(p string) bool { return w.existing[p] }

func (w *fakeSetupWorld) Copy(src, dst string) error {
	w.ops = append(w.ops, "copy "+src+" "+dst)
	return nil
}

func (w *fakeSetupWorld) Symlink(target, link string) error {
	w.ops = append(w.ops, "symlink "+target+" "+link)
	return nil
}

func (w *fakeSetupWorld) CloneTree(_ context.Context, src, dst string) error {
	w.ops = append(w.ops, "clone "+src+" "+dst)
	return nil
}

func (w *fakeSetupWorld) Lockfile(dir string) (domain.Lockfile, error) { return w.locks[dir], nil }

func (w *fakeSetupWorld) FreeBytes(string) (int64, error) {
	v := w.free[0]
	w.free = w.free[1:]
	return v, nil
}

func (w *fakeSetupWorld) Run(_ context.Context, dir string, argv ...string) error {
	line := "run " + strings.Join(argv, " ") + " in " + dir
	w.ops = append(w.ops, line)
	return w.runErr[strings.Join(argv, " ")]
}

func (w *fakeSetupWorld) now() time.Time {
	w.ticks++
	return time.Unix(0, 0).Add(time.Duration(w.ticks) * 1500 * time.Millisecond)
}

func (w *fakeSetupWorld) setup() app.WorktreeSetup {
	return app.WorktreeSetup{Recipes: w, FS: w, Runner: w, Git: w, Now: w.now}
}

func newWorld(recipe *domain.Recipe) *fakeSetupWorld {
	return &fakeSetupWorld{
		recipe:   recipe,
		main:     "/repos/api",
		existing: map[string]bool{},
		locks:    map[string]domain.Lockfile{},
		free:     []int64{10_000_000_000, 9_990_000_000},
	}
}

type fakeLister map[string]domain.RepoListing

func (f fakeLister) ListWorktrees(_ context.Context, dir string) (domain.RepoListing, error) {
	l, ok := f[dir]
	if !ok {
		return domain.RepoListing{}, errors.New("not a git repo")
	}
	return l, nil
}

type fakeFinder struct {
	mu    sync.Mutex
	prs   map[string][]domain.PullRequest
	calls map[string]int
}

func (f *fakeFinder) PRs(_ context.Context, repo string) ([]domain.PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[repo]++
	prs, ok := f.prs[repo]
	if !ok {
		return nil, errors.New("gh failed")
	}
	return prs, nil
}

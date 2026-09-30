package app_test

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

type fakeHost struct {
	app.TerminalHost
	panes   []app.PaneInfo
	listErr error
}

func (f fakeHost) List(context.Context) ([]app.PaneInfo, error) { return f.panes, f.listErr }

type fakeStore struct {
	bindings []app.PaneBinding
	idled    []string
}

func (s *fakeStore) PaneBindings(context.Context) ([]app.PaneBinding, error) {
	return s.bindings, nil
}

func (s *fakeStore) MarkIdle(_ context.Context, sessionID string) error {
	s.idled = append(s.idled, sessionID)
	return nil
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

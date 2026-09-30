package daemon_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

type memStore struct {
	mu   sync.Mutex
	snap app.Snapshot
}

func (s *memStore) PutWorkspace(w domain.Workspace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Workspaces = append(s.snap.Workspaces, w)
}

func (s *memStore) DeleteWorkspace(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.snap.Workspaces[:0]
	for _, w := range s.snap.Workspaces {
		if w.Root != root {
			kept = append(kept, w)
		}
	}
	s.snap.Workspaces = kept
}

func (s *memStore) PutTask(x domain.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Tasks = append(s.snap.Tasks, x)
}

func (s *memStore) PutWorktree(w domain.Worktree) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Worktrees = append(s.snap.Worktrees, w)
}

func (s *memStore) DeleteWorktree(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.snap.Worktrees[:0]
	for _, w := range s.snap.Worktrees {
		if w.ID != id {
			kept = append(kept, w)
		}
	}
	s.snap.Worktrees = kept
}

func (s *memStore) PutSession(x domain.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Sessions = append(s.snap.Sessions, x)
}

func (s *memStore) PutEvent(ev domain.SessionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Events = append(s.snap.Events, ev)
}

func (s *memStore) Load() (app.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap, nil
}

func (s *memStore) Flush() error { return nil }
func (s *memStore) Close() error { return nil }

type fakeFS struct {
	gate     chan struct{}
	markers  map[string]domain.GitMarker
	children map[string][]domain.Child
}

func (f fakeFS) Marker(path string) (domain.GitMarker, error) {
	if f.gate != nil {
		<-f.gate
	}
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

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Minute)
	return c.now
}

type liveGit struct{ branch atomic.Value }

func (g *liveGit) Inspect(context.Context, string) (app.RepoFacts, error) {
	return app.RepoFacts{Branch: g.branch.Load().(string)}, nil
}

type fakeClientHost struct {
	mu      sync.Mutex
	opened  []app.PaneSpec
	open    map[app.Slot]bool
	focused []app.Slot
}

func (h *fakeClientHost) OpenClient(_ context.Context, name string, tui app.PaneSpec) (app.Slot, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.opened = append(h.opened, tui)
	slot := app.Slot("@" + name + string(rune('0'+len(h.opened))))
	if h.open == nil {
		h.open = map[app.Slot]bool{}
	}
	h.open[slot] = true
	return slot, nil
}

func (h *fakeClientHost) ClientOpen(_ context.Context, slot app.Slot) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.open[slot]
}

func (h *fakeClientHost) AttachCommand(slot app.Slot) []string {
	return []string{"tmux", "attach", "-t", string(slot)}
}

func (h *fakeClientHost) FocusSlot(_ context.Context, slot app.Slot) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.open[slot] {
		return errors.New("no such slot")
	}
	h.focused = append(h.focused, slot)
	return nil
}

func (h *fakeClientHost) close(slot app.Slot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.open, slot)
}

type fakeHost struct {
	app.TerminalHost
	mu    sync.Mutex
	specs []app.PaneSpec
	err   error
}

func (h *fakeHost) Create(_ context.Context, spec app.PaneSpec) (app.PaneID, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.err != nil {
		return "", h.err
	}
	h.specs = append(h.specs, spec)
	return "%7", nil
}

type fakeNotifier struct{ banners chan domain.Banner }

func newFakeNotifier() *fakeNotifier { return &fakeNotifier{banners: make(chan domain.Banner, 64)} }

func (n *fakeNotifier) Notify(_ context.Context, b domain.Banner) error {
	n.banners <- b
	return nil
}

type fakeForeground struct{ terminal atomic.Bool }

func (f *fakeForeground) TerminalFrontmost(context.Context) bool { return f.terminal.Load() }

type fakeLister struct {
	mu       sync.Mutex
	listings map[string]domain.RepoListing
}

func (f *fakeLister) ListWorktrees(_ context.Context, dir string) (domain.RepoListing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.listings[dir]
	if !ok {
		return domain.RepoListing{}, errors.New("not a git repo")
	}
	return l, nil
}

func (f *fakeLister) set(dir string, wts ...domain.ListedWorktree) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listings[dir] = domain.RepoListing{Main: dir, Worktrees: wts}
}

type fakeFinder struct {
	mu  sync.Mutex
	prs map[string][]domain.PullRequest
}

func (f *fakeFinder) PRs(_ context.Context, repo string) ([]domain.PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prs[repo], nil
}

func (f *fakeFinder) set(repo string, prs ...domain.PullRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prs[repo] = prs
}

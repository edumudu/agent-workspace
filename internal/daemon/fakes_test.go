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

func (s *memStore) PutSession(x domain.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Sessions = append(s.snap.Sessions, x)
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

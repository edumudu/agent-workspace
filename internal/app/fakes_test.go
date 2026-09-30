package app_test

import (
	"context"
	"errors"

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

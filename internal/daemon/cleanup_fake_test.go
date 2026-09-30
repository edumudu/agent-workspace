package daemon_test

import (
	"context"
	"sync"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

// fakeCleanupWorld reports every worktree clean and long idle, nobody inside
// it, and records what gets moved to the trash.
type fakeCleanupWorld struct {
	mu    sync.Mutex
	moved []string
}

func (f *fakeCleanupWorld) CleanupFacts(context.Context, domain.Worktree) (app.WorktreeGitFacts, error) {
	return app.WorktreeGitFacts{Fingerprint: "f"}, nil
}

func (f *fakeCleanupWorld) Backup(context.Context, domain.Worktree, string) error { return nil }

func (f *fakeCleanupWorld) CreateBranch(_ context.Context, _ domain.Worktree, name string) (string, error) {
	return name, nil
}

func (f *fakeCleanupWorld) Prune(context.Context, string) error { return nil }

func (f *fakeCleanupWorld) Holders(context.Context, []string) (map[string][]string, error) {
	return map[string][]string{}, nil
}

func (f *fakeCleanupWorld) Move(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.moved = append(f.moved, path)
	return nil
}

func (f *fakeCleanupWorld) Purge() {}

func (f *fakeCleanupWorld) Record(app.CleanupRecord) {}

func (f *fakeCleanupWorld) movedPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.moved...)
}

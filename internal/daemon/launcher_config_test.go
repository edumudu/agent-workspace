package daemon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/daemon"
)

func TestLauncherMaxParallelComesFromTheLauncherTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if got, err := daemon.LoadMaxParallel(path); err != nil || got != 0 {
		t.Fatalf("missing file: %d, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("[theme]\nname = \"latte\"\n[launcher]\nmax_parallel = 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := daemon.LoadMaxParallel(path); err != nil || got != 5 {
		t.Fatalf("got %d, %v, want 5", got, err)
	}
	if err := os.WriteFile(path, []byte("[launcher\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.LoadMaxParallel(path); err == nil {
		t.Fatal("a broken file gave no error")
	}
}

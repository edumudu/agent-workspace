package claude_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/adapters/claude"
)

func TestReadDefaultsTakesModelAndEffortFromSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"model":"opus","effortLevel":"low","hooks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	model, effort, err := claude.ReadDefaults(path)
	if err != nil || model != "opus" || effort != "low" {
		t.Fatalf("got %q, %q, %v", model, effort, err)
	}
	model, effort, err = claude.ReadDefaults(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || model != "" || effort != "" {
		t.Fatalf("missing file: %q, %q, %v", model, effort, err)
	}
}

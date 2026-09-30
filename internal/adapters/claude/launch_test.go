package claude_test

import (
	"reflect"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/adapters/claude"
	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestLaunchRunsClaudeWithTheModelAndEffortInTheSessionDir(t *testing.T) {
	var a app.HarnessAdapter = claude.Adapter{}
	if a.Harness() != domain.HarnessClaude {
		t.Fatalf("harness %q", a.Harness())
	}
	got := a.Launch(app.LaunchRequest{Name: "api-42", Dir: "/home/dev/api", Model: "sonnet", Effort: "high"})
	want := app.PaneSpec{Name: "api-42", Dir: "/home/dev/api", Command: []string{"claude", "--model", "sonnet", "--effort", "high"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLaunchLeavesDefaultsToClaude(t *testing.T) {
	got := claude.Adapter{Binary: "/opt/claude"}.Launch(app.LaunchRequest{Dir: "/w"})
	if !reflect.DeepEqual(got.Command, []string{"/opt/claude"}) {
		t.Fatalf("command %v", got.Command)
	}
}

package codex

import (
	"slices"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

var _ app.HarnessAdapter = Adapter{}

func TestLaunchStartsCodexWithModelAndEffortInTheWorkspace(t *testing.T) {
	spec := Adapter{}.Launch(app.LaunchRequest{Name: "api-42", Dir: "/work/api", Model: "gpt-6.1-sol", Effort: "high"})
	want := []string{"codex", "--model", "gpt-6.1-sol", "-c", `model_reasoning_effort="high"`}
	if !slices.Equal(spec.Command, want) || spec.Dir != "/work/api" || spec.Name != "api-42" {
		t.Fatalf("spec %+v", spec)
	}
	if (Adapter{}).Harness() != domain.HarnessCodex {
		t.Fatal("wrong harness")
	}
}

func TestLaunchOmitsWhatWasNotChosenAndPassesThePromptLast(t *testing.T) {
	spec := Adapter{}.Launch(app.LaunchRequest{Name: "n", Dir: "/w", Prompt: "--fix the build"})
	want := []string{"codex", "--", "--fix the build"}
	if !slices.Equal(spec.Command, want) {
		t.Fatalf("command %v", spec.Command)
	}
	bare := Adapter{Binary: "/bin/fake-codex"}.Launch(app.LaunchRequest{Name: "n", Dir: "/w"})
	if !slices.Equal(bare.Command, []string{"/bin/fake-codex"}) {
		t.Fatalf("command %v", bare.Command)
	}
}

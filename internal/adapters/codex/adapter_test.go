package codex

import (
	"reflect"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestAdapterLaunchesCodexAsTheCodexHarness(t *testing.T) {
	var a app.HarnessAdapter = Adapter{}
	if a.Harness() != domain.HarnessCodex {
		t.Fatalf("harness %q", a.Harness())
	}
	got := a.Launch(app.LaunchRequest{Name: "api", Dir: "/w/api", Model: "gpt-6"})
	want := app.PaneSpec{Name: "api", Dir: "/w/api", Command: []string{"codex", "--model", "gpt-6"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

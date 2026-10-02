package tui_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/tui"
)

func ompState() rpc.State {
	st := fixture(1, 0)
	st.Sessions[0].Harness, st.Sessions[0].Model, st.Sessions[0].Effort = domain.HarnessOmp, "anthropic/opus", "high"
	return st
}

func TestNewSessionDialogCyclesEveryCatalogHarness(t *testing.T) {
	m, c := dialogModel(t, withWorkspaces(fixture(1, 0)))
	m = typeText(m, "add search")
	m = pressCmd(pressCmd(m, keyTab), keyTab)
	m = pressCmd(pressCmd(m, keyRight), keyRight)
	if out := screen(m); !strings.Contains(out, "omp") {
		t.Fatalf("two steps right is not omp:\n%s", out)
	}
	if got := startedParams(t, m, c).Harness; got != "omp" {
		t.Fatalf("harness %q", got)
	}
	c.calls = nil
	if got := startedParams(t, pressCmd(m, keyRight), c).Harness; got != "claude" {
		t.Fatalf("after omp came %q, want claude", got)
	}
}

func TestNewSessionDialogTakesATypedModelForOmp(t *testing.T) {
	m, c := dialogModel(t, withWorkspaces(fixture(1, 0)))
	m = typeText(m, "add search")
	m = pressCmd(pressCmd(m, keyTab), keyTab)
	m = pressCmd(pressCmd(m, keyLeft), keyTab)
	m = typeText(m, "anthropic/opus-x")
	m = pressCmd(m, keyBack)
	got := startedParams(t, m, c)
	want := rpc.NewSessionParams{Workspace: "/src/shop", WorkItem: "add search", Harness: "omp", Model: "anthropic/opus-"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("params %+v, want %+v", got, want)
	}
}

func TestSidebarTagsAnOmpSessionOM(t *testing.T) {
	st := ompState()
	out := screen(newModel(&st, nil))
	if !strings.Contains(out, " OM") || strings.Contains(out, " CC") {
		t.Fatalf("omp row:\n%s", ansi.Strip(out))
	}
}

func TestModelSwitchMOnOmpTakesATypedModel(t *testing.T) {
	st := ompState()
	sw := &fakeSwitcher{}
	m := press(switchModel(&st, sw), "M", "o", "p", "u", "s", "x")
	m = pressCmd(m, keyBack)
	next, cmd := m.Update(keyEnter)
	if cmd == nil {
		t.Fatalf("enter returned no command:\n%s", screen(m))
	}
	cmd()
	if len(sw.calls) != 1 || sw.calls[0] != "s01 model opus" {
		t.Fatalf("calls %q", sw.calls)
	}
	if out := screen(next.(tui.Model)); strings.Contains(out, "MODEL") {
		t.Fatalf("picker still open:\n%s", out)
	}
}

func TestModelSwitchDefaultsReadOmpFromConfig(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	if err := os.WriteFile(path, []byte("[defaults.omp]\nmodel = \"anthropic/opus\"\neffort = \"max\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := tui.LoadDefaults(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := d[domain.HarnessOmp]; got.Model != "anthropic/opus" || got.Effort != "max" {
		t.Fatalf("omp %+v", got)
	}
}

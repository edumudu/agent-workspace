package tui_test

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/tui"
)

var keyShiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}

func defaultsDialog(t *testing.T) (tui.Model, *fakeCaller) {
	t.Helper()
	c := &fakeCaller{}
	m := tui.New(tui.Options{Theme: tui.Latte(), Now: clock, Calls: c, Defaults: map[domain.Harness]tui.Defaults{
		domain.HarnessClaude: {Model: "sonnet", Effort: "high"},
		domain.HarnessCodex:  {Model: "gpt-5", Effort: "medium"},
	}})
	m = update(m, tea.WindowSizeMsg{Width: 48, Height: 40})
	m = update(m, tui.StateMsg(withWorkspaces(fixture(1, 0))))
	return typeText(press(m, "n"), "fix the login bug"), c
}

func startedParams(t *testing.T, m tui.Model, c *fakeCaller) rpc.NewSessionParams {
	t.Helper()
	pressCmd(m, keyEnter)
	if len(c.calls) == 0 {
		t.Fatal("no call")
	}
	return c.calls[0].params.(rpc.NewSessionParams)
}

func TestModelSwitchDialogStartsWithTheHarnessDefaults(t *testing.T) {
	m, c := defaultsDialog(t)
	if out := screen(m); !strings.Contains(out, "sonnet") {
		t.Fatalf("model not prefilled:\n%s", out)
	}
	got := startedParams(t, m, c)
	if got.Harness != "claude" || got.Model != "sonnet" || got.Effort != "high" {
		t.Fatalf("params %+v", got)
	}
}

func TestModelSwitchDialogFollowsTheDefaultsWhenTheHarnessChanges(t *testing.T) {
	m, c := defaultsDialog(t)
	m = pressCmd(pressCmd(pressCmd(m, keyTab), keyTab), keyRight)
	got := startedParams(t, m, c)
	if got.Harness != "codex" || got.Model != "gpt-5" || got.Effort != "medium" {
		t.Fatalf("codex params %+v", got)
	}
	m = pressCmd(m, keyLeft)
	c.calls = nil
	got = startedParams(t, m, c)
	if got.Harness != "claude" || got.Model != "sonnet" || got.Effort != "high" {
		t.Fatalf("back on claude %+v", got)
	}
}

func TestModelSwitchDialogKeepsAModelTheUserTyped(t *testing.T) {
	m, c := defaultsDialog(t)
	m = pressCmd(pressCmd(pressCmd(m, keyTab), keyTab), keyTab)
	for range len("sonnet") {
		m = update(m, keyBack)
	}
	m = typeText(m, "custom")
	m = pressCmd(pressCmd(m, keyShiftTab), keyRight)
	got := startedParams(t, m, c)
	want := rpc.NewSessionParams{Workspace: got.Workspace, WorkItem: "fix the login bug", Harness: "codex", Model: "custom", Effort: "medium"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("params %+v, want %+v", got, want)
	}
}

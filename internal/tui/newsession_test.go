package tui_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/tui"
)

func withWorkspaces(st rpc.State) rpc.State {
	st.Workspaces = []domain.Workspace{
		{Root: "/src/api", Kind: domain.WorkspaceSingle, LastUsed: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{Root: "/src/shop", Kind: domain.WorkspaceOrchestration, LastUsed: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)},
	}
	return st
}

// run drains cmd, feeding every message it yields back into m.
func run(m tui.Model, cmd tea.Cmd) tui.Model {
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			return m
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(tui.Model)
	}
	return m
}

func typeText(m tui.Model, s string) tui.Model {
	return update(m, tea.PasteMsg{Content: s})
}

func pressCmd(m tui.Model, k tea.KeyPressMsg) tui.Model {
	next, cmd := m.Update(k)
	return run(next.(tui.Model), cmd)
}

var (
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyRight = tea.KeyPressMsg{Code: tea.KeyRight}
	keyLeft  = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyTab   = tea.KeyPressMsg{Code: tea.KeyTab}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyBack  = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

func dialogModel(t *testing.T, st rpc.State) (tui.Model, *fakeCaller) {
	t.Helper()
	c := &fakeCaller{}
	m := tui.New(tui.Options{Theme: tui.Latte(), Now: clock, Calls: c})
	m = update(m, tea.WindowSizeMsg{Width: 48, Height: 40})
	m = update(m, tui.StateMsg(st))
	return press(m, "n"), c
}

func TestNewSessionDialogStartsTheSessionAndShowsItsPane(t *testing.T) {
	m, c := dialogModel(t, withWorkspaces(fixture(1, 0)))
	out := screen(m)
	if !strings.Contains(out, "NEW SESSION") || !strings.Contains(out, "shop") {
		t.Fatalf("dialog does not default to the last used workspace:\n%s", out)
	}
	m = typeText(m, "https://linear.app/acme/issue/ENG-9/add-search")
	if out := screen(m); !strings.Contains(out, "linear ENG-9") {
		t.Fatalf("dialog does not say what the work item is:\n%s", out)
	}
	m = pressCmd(m, keyTab)
	m = pressCmd(m, keyLeft)
	m = pressCmd(m, keyTab)
	m = pressCmd(m, keyRight)
	m = pressCmd(m, keyTab)
	m = typeText(m, "gpt-6x")
	m = update(m, keyBack)
	m = pressCmd(m, keyTab)
	m = pressCmd(m, keyRight)
	m = pressCmd(m, keyRight)
	m = pressCmd(m, keyEnter)

	if got := c.methods(); !slices.Equal(got, []string{rpc.MethodNewSession, rpc.MethodSessionFocus}) {
		t.Fatalf("calls %v", got)
	}
	want := rpc.NewSessionParams{Workspace: "/src/api", WorkItem: "https://linear.app/acme/issue/ENG-9/add-search", Harness: "codex", Model: "gpt-6", Effort: "medium"}
	if !reflect.DeepEqual(c.calls[0].params, want) {
		t.Fatalf("params %+v, want %+v", c.calls[0].params, want)
	}
	if !reflect.DeepEqual(c.calls[1].params, rpc.SessionFocusParams{ID: "new1"}) {
		t.Fatalf("focus %+v", c.calls[1].params)
	}
	if out := screen(m); strings.Contains(out, "NEW SESSION") {
		t.Fatalf("dialog still open after start:\n%s", out)
	}
}

func TestNewSessionDialogCyclesWorkspacesBothWays(t *testing.T) {
	m, _ := dialogModel(t, withWorkspaces(rpc.State{}))
	m = pressCmd(typeText(m, "x"), keyTab)
	if out := screen(pressCmd(m, keyRight)); !strings.Contains(out, "‹ api ›") {
		t.Fatalf("right from the last workspace should wrap to api:\n%s", out)
	}
	if out := screen(pressCmd(pressCmd(m, keyLeft), keyLeft)); !strings.Contains(out, "‹ shop ›") {
		t.Fatalf("left twice should come back to shop:\n%s", out)
	}
}

func TestNewSessionDialogEscapeCancelsWithoutCalling(t *testing.T) {
	m, c := dialogModel(t, withWorkspaces(rpc.State{}))
	m = pressCmd(typeText(m, "fix it"), keyEsc)
	if len(c.methods()) != 0 || strings.Contains(screen(m), "NEW SESSION") {
		t.Fatalf("calls %v after esc:\n%s", c.methods(), screen(m))
	}
	if m = press(m, "j"); strings.Contains(screen(m), "fix it") {
		t.Fatal("keys after esc still type into the dialog")
	}
}

func TestNewSessionDialogKeepsABrokenURLAsText(t *testing.T) {
	m, c := dialogModel(t, withWorkspaces(rpc.State{}))
	m = typeText(m, "https://linear.app/acme/nope")
	if out := screen(m); !strings.Contains(out, "text") {
		t.Fatalf("broken URL not shown as text:\n%s", out)
	}
	pressCmd(m, keyEnter)
	if len(c.calls) == 0 || c.calls[0].params.(rpc.NewSessionParams).WorkItem != "https://linear.app/acme/nope" {
		t.Fatalf("calls %+v", c.calls)
	}
}

func TestNewSessionDialogNeedsAWorkItemAndAWorkspace(t *testing.T) {
	m, c := dialogModel(t, withWorkspaces(rpc.State{}))
	m = pressCmd(m, keyEnter)
	if len(c.methods()) != 0 || !strings.Contains(screen(m), "work item is empty") {
		t.Fatalf("empty work item: calls %v\n%s", c.methods(), screen(m))
	}
	m, c = dialogModel(t, rpc.State{})
	m = pressCmd(typeText(m, "x"), keyEnter)
	if len(c.methods()) != 0 || !strings.Contains(screen(m), "workspace add") {
		t.Fatalf("no workspace: calls %v\n%s", c.methods(), screen(m))
	}
}

func TestNewSessionFailureKeepsTheDialogAndSaysWhy(t *testing.T) {
	m, c := dialogModel(t, withWorkspaces(rpc.State{}))
	c.err = errors.New("failed: branch exists")
	m = pressCmd(typeText(m, "x"), keyEnter)
	if out := screen(m); !strings.Contains(out, "NEW SESSION") || !strings.Contains(out, "branch exists") {
		t.Fatalf("failure not shown in the dialog:\n%s", out)
	}
}

func TestXEndsTheSelectedSessionAfterYes(t *testing.T) {
	st := fixture(2, 0)
	c := &fakeCaller{}
	m := tui.New(tui.Options{Theme: tui.Latte(), Now: clock, Calls: c})
	m = update(m, tea.WindowSizeMsg{Width: 48, Height: 40})
	m = update(m, tui.StateMsg(st))
	m = press(m, "x")
	if !strings.Contains(screen(m), "end session 1?") {
		t.Fatalf("no confirmation:\n%s", screen(m))
	}
	m = pressCmd(m, key("n"))
	if len(c.methods()) != 0 {
		t.Fatalf("n still ended: %v", c.methods())
	}
	m = press(m, "x")
	pressCmd(m, key("y"))
	if !slices.Equal(c.methods(), []string{rpc.MethodEndSession}) || !reflect.DeepEqual(c.calls[0].params, rpc.SessionRef{ID: m.Selected()}) {
		t.Fatalf("calls %+v", c.calls)
	}
}

func lowClaudeState() rpc.State {
	st := withWorkspaces(rpc.State{})
	st.Sessions = []domain.Session{
		{ID: "c", Harness: domain.HarnessClaude, LimitsAt: clock(), Limits: []domain.RateLimit{{Window: "five_hour", UsedPercent: 90}}},
		{ID: "x", Harness: domain.HarnessCodex, LimitsAt: clock(), Limits: []domain.RateLimit{{Window: "five_hour", UsedPercent: 36}}},
	}
	return st
}

var keyCtrlS = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}

func TestNewSessionDialogWarnsOnLowQuotaAndSwitchesHarness(t *testing.T) {
	m, c := dialogModel(t, lowClaudeState())
	out := screen(m)
	if !strings.Contains(out, "claude 5h 10% left") || !strings.Contains(out, "ctrl+s codex 64%") {
		t.Fatalf("no low-quota warning with a switch:\n%s", out)
	}
	m = pressCmd(m, keyCtrlS)
	if out := screen(m); !strings.Contains(out, "‹ codex ›") || strings.Contains(out, "10% left") {
		t.Fatalf("ctrl+s did not switch to codex:\n%s", out)
	}
	pressCmd(typeText(m, "x"), keyEnter)
	if len(c.calls) == 0 || c.calls[0].params.(rpc.NewSessionParams).Harness != "codex" {
		t.Fatalf("calls %+v", c.calls)
	}
}

func TestNewSessionDialogWarnsWithoutASwitchWhenTheOtherHarnessIsUnknown(t *testing.T) {
	st := lowClaudeState()
	st.Sessions = st.Sessions[:1]
	m, _ := dialogModel(t, st)
	out := screen(m)
	if !strings.Contains(out, "claude 5h 10% left") || strings.Contains(out, "ctrl+s") {
		t.Fatalf("warning should have no switch:\n%s", out)
	}
	if out := screen(pressCmd(m, keyCtrlS)); !strings.Contains(out, "‹ claude ›") {
		t.Fatalf("ctrl+s switched with nothing to switch to:\n%s", out)
	}
}

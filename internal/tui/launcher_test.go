package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/tui"
)

const pasted = "https://linear.app/acme/issue/ENG-1/first\nhttps://linear.app/acme/issue/ENG-2/second\nnot a url"

func launcherModel(t *testing.T, st rpc.State, opts tui.Options) (tui.Model, *fakeCaller) {
	t.Helper()
	c := &fakeCaller{}
	opts.Theme, opts.Now, opts.Calls = tui.Latte(), clock, c
	m := tui.New(opts)
	m = update(m, tea.WindowSizeMsg{Width: 60, Height: 40})
	return update(m, tui.StateMsg(st)), c
}

func TestLauncherInputCountsIssuesAndSendsThemWithTheClaudeDefaults(t *testing.T) {
	opts := tui.Options{Defaults: map[domain.Harness]tui.Defaults{domain.HarnessClaude: {Model: "opus", Effort: "high"}}}
	m, c := launcherModel(t, withWorkspaces(rpc.State{}), opts)
	m = press(m, "L")
	m = typeText(m, pasted)
	out := screen(m)
	for _, want := range []string{"LAUNCH", "ENG-1/first", "2 issues", "1 not a Linear issue"} {
		if !strings.Contains(out, want) {
			t.Fatalf("input does not show %q:\n%s", want, out)
		}
	}
	m = pressCmd(m, keyEnter)
	if len(c.calls) != 1 || c.calls[0].method != rpc.MethodLauncherEnqueue {
		t.Fatalf("calls %+v", c.calls)
	}
	got := c.calls[0].params.(rpc.LauncherEnqueueParams)
	if got.Input != pasted || got.Harness != "claude" || got.Model != "opus" || got.Effort != "high" {
		t.Fatalf("params %+v", got)
	}
	if strings.Contains(screen(m), "LAUNCH") {
		t.Fatalf("input still open after sending:\n%s", screen(m))
	}
}

func TestLauncherInputTakesTypedLinesAndEditsThem(t *testing.T) {
	m, _ := launcherModel(t, withWorkspaces(rpc.State{}), tui.Options{})
	m = press(m, "L")
	m = typeText(m, "https://linear.app/acme/issue/ENG-1")
	m = update(m, tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = typeText(m, "https://linear.app/acme/issue/ENG-2")
	if out := screen(m); !strings.Contains(out, "2 issues") {
		t.Fatalf("a typed newline did not start a second issue:\n%s", out)
	}
	m = update(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if out := screen(m); strings.Contains(out, "ENG-") {
		t.Fatalf("ctrl+u did not clear:\n%s", out)
	}
}

func TestLauncherInputWithoutAnIssueSendsNothingAndEscCancels(t *testing.T) {
	m, c := launcherModel(t, withWorkspaces(rpc.State{}), tui.Options{})
	m = press(m, "L")
	m = typeText(m, "fix the login page")
	m = pressCmd(m, keyEnter)
	if len(c.calls) != 0 {
		t.Fatalf("sent %+v without an issue", c.calls)
	}
	if out := screen(m); !strings.Contains(out, "LAUNCH") || !strings.Contains(out, "no Linear issue") {
		t.Fatalf("no refusal shown:\n%s", out)
	}
	m = pressCmd(m, keyEsc)
	if out := screen(m); strings.Contains(out, "LAUNCH") {
		t.Fatalf("esc did not close:\n%s", out)
	}
}

func queueState(items ...domain.LaunchItem) rpc.State {
	st := lowClaudeState()
	st.Queue = items
	return st
}

var waiting = []domain.LaunchItem{
	{ID: "q1", Ref: "ENG-4", Request: domain.StartRequest{Harness: domain.HarnessClaude, Model: "opus", Effort: "high"}},
	{ID: "q2", Ref: "ENG-5", Request: domain.StartRequest{Harness: domain.HarnessClaude, Model: "opus"}},
	{ID: "q3", Ref: "ENG-3", Starting: true, Request: domain.StartRequest{Harness: domain.HarnessClaude}},
	{ID: "q4", Ref: "ENG-2", Err: "tmux down", Request: domain.StartRequest{Harness: domain.HarnessClaude}},
}

func TestLauncherQueueShowsEachIssueWithItsStateAndTheCodexOffer(t *testing.T) {
	opts := tui.Options{Fallback: domain.FallbackConfig{Models: map[string]string{"opus": "gpt-5"}}}
	m, _ := launcherModel(t, queueState(waiting...), opts)
	out := screen(m)
	for _, want := range []string{"QUEUE", "ENG-4", "ENG-5", "starting", "tmux down", "codex 64% gpt-5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("queue does not show %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "codex 64% gpt-5"); n != 2 {
		t.Fatalf("offer shown %d times, want once per waiting issue:\n%s", n, out)
	}
}

func TestLauncherQueueHasNoOfferWhileClaudeHasQuota(t *testing.T) {
	st := withWorkspaces(rpc.State{})
	st.Queue = waiting
	m, _ := launcherModel(t, st, tui.Options{})
	out := screen(m)
	if !strings.Contains(out, "ENG-4") || strings.Contains(out, "codex") {
		t.Fatalf("queue with no low quota:\n%s", out)
	}
}

func TestLauncherQueueDiffReplacesTheQueue(t *testing.T) {
	m, _ := launcherModel(t, queueState(waiting...), tui.Options{})
	next := []domain.LaunchItem{{ID: "q9", Ref: "ENG-9"}}
	m = update(m, tui.DiffMsg(rpc.Diff{Queue: &next}))
	out := screen(m)
	if !strings.Contains(out, "ENG-9") || strings.Contains(out, "ENG-4") {
		t.Fatalf("diff did not replace the queue:\n%s", out)
	}
	empty := []domain.LaunchItem{}
	m = update(m, tui.DiffMsg(rpc.Diff{Queue: &empty}))
	if out := screen(m); strings.Contains(out, "QUEUE") {
		t.Fatalf("empty queue still shown:\n%s", out)
	}
}

func TestLauncherCMovesEveryOfferedIssueToCodex(t *testing.T) {
	opts := tui.Options{Fallback: domain.FallbackConfig{Models: map[string]string{"opus": "gpt-5"}, Efforts: map[string]string{"high": "medium"}}}
	m, c := launcherModel(t, queueState(waiting...), opts)
	pressCmd(m, key("c"))
	want := []rpc.LauncherRetargetParams{
		{ID: "q1", Harness: "codex", Model: "gpt-5", Effort: "medium"},
		{ID: "q2", Harness: "codex", Model: "gpt-5"},
	}
	if len(c.calls) != len(want) {
		t.Fatalf("calls %+v", c.calls)
	}
	for i, w := range want {
		if c.calls[i].method != rpc.MethodLauncherRetarget || c.calls[i].params != w {
			t.Fatalf("call %d: %+v, want %+v", i, c.calls[i], w)
		}
	}
}

func TestLauncherCDoesNothingWithoutAnOffer(t *testing.T) {
	st := withWorkspaces(rpc.State{})
	st.Queue = waiting
	m, c := launcherModel(t, st, tui.Options{})
	pressCmd(m, key("c"))
	if len(c.calls) != 0 {
		t.Fatalf("calls %+v", c.calls)
	}
}

func TestLauncherXDropsTheWaitingAndFailedIssuesButNotTheStartingOne(t *testing.T) {
	m, c := launcherModel(t, queueState(waiting...), tui.Options{})
	pressCmd(m, key("X"))
	var dropped []string
	for _, call := range c.calls {
		if call.method != rpc.MethodLauncherDrop {
			t.Fatalf("call %+v", call)
		}
		dropped = append(dropped, call.params.(rpc.LauncherItemRef).ID)
	}
	if strings.Join(dropped, " ") != "q1 q2 q4" {
		t.Fatalf("dropped %v", dropped)
	}
}

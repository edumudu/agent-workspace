package tui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/tui"
)

func agent(session, id, kind string, state domain.SubagentState, summary string) domain.Subagent {
	return domain.Subagent{SessionID: session, ID: id, Type: kind, State: state, Summary: summary}
}

func subagentFixture() rpc.State {
	st := fixture(3, 1)
	st.Subagents = []domain.Subagent{
		agent("s02", "a1", "Explore", domain.SubagentStopped, "Found 3 callers."),
		agent("s02", "a2", "Plan", domain.SubagentRunning, ""),
		{SessionID: "s02", ID: "a3", ParentID: "a2", Type: "Reviewer", State: domain.SubagentRunning},
	}
	return st
}

func TestSessionListsItsSubagentsAsATree(t *testing.T) {
	st := subagentFixture()
	out := screen(newModel(&st, nil))
	for _, want := range []string{"Explore", "Found 3 callers.", "Plan", "Reviewer"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen missing %q:\n%s", want, out)
		}
	}
	lines := strings.Split(out, "\n")
	indent := func(label string) int {
		for _, l := range lines {
			if i := strings.Index(l, label); i >= 0 {
				return i
			}
		}
		t.Fatalf("no line with %q", label)
		return 0
	}
	if indent("Reviewer") <= indent("Plan") {
		t.Errorf("Reviewer is not nested under Plan:\n%s", out)
	}
	if indent("Plan") != indent("Explore") {
		t.Errorf("Plan and Explore are not siblings:\n%s", out)
	}
}

func TestStoppedSubagentsLookDifferentFromRunningOnes(t *testing.T) {
	st := subagentFixture()
	out := screen(newModel(&st, nil))
	line := func(label string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, label) {
				return l
			}
		}
		return ""
	}
	if !strings.Contains(line("Explore"), "✓") || strings.Contains(line("Plan"), "✓") {
		t.Errorf("stopped and running rows do not differ:\n%s", out)
	}
}

func TestSessionsWithoutSubagentsShowNoTree(t *testing.T) {
	st := fixture(3, 1)
	if out := screen(newModel(&st, nil)); strings.Contains(out, "✓") {
		t.Errorf("screen has subagent rows:\n%s", out)
	}
}

func TestSubagentDiffUpdatesTheTree(t *testing.T) {
	st := subagentFixture()
	m := newModel(&st, nil)
	stopped := agent("s02", "a2", "Plan", domain.SubagentStopped, "Plan is ready.")
	m = update(m, tui.DiffMsg(rpc.Diff{Subagent: &stopped}))
	out := screen(m)
	if !strings.Contains(out, "Plan is ready.") {
		t.Errorf("stopped summary missing:\n%s", out)
	}
	fresh := agent("s02", "a4", "Explore", domain.SubagentRunning, "")
	out = screen(update(m, tui.DiffMsg(rpc.Diff{Subagent: &fresh})))
	if strings.Count(out, "Explore") != 2 {
		t.Errorf("new subagent missing:\n%s", out)
	}
}

func TestCollapsingASessionHidesItsSubagents(t *testing.T) {
	st := subagentFixture()
	m := selectSession(t, newModel(&st, nil), "s02")
	if out := screen(press(m, "o")); strings.Contains(out, "Reviewer") {
		t.Errorf("collapsed session still shows subagents:\n%s", out)
	}
}

func TestLongSubagentListIsCutWithACount(t *testing.T) {
	st := fixture(1, 1)
	for i := range 12 {
		st.Subagents = append(st.Subagents, agent("s01", fmt.Sprint("a", i), fmt.Sprintf("Agent%02d", i), domain.SubagentRunning, ""))
	}
	out := screen(newModel(&st, nil))
	if !strings.Contains(out, "Agent07") || strings.Contains(out, "Agent08") || !strings.Contains(out, "4 more") {
		t.Errorf("list not cut at 8:\n%s", out)
	}
}

func TestSubagentTextCannotRepaintTheTerminal(t *testing.T) {
	st := fixture(1, 1)
	st.Subagents = []domain.Subagent{agent("s01", "a1", "Explore\x1b[2J", domain.SubagentStopped, "done\x1b[31m red")}
	if out := newModel(&st, nil).View().Content; strings.Contains(out, "\x1b[2J") || strings.Contains(out, "\x1b[31m red") {
		t.Errorf("escape sequence reached the screen: %q", out)
	}
}

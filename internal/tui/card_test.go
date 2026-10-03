package tui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/tui"
)

func toolEvent(session, tool, detail string) domain.SessionEvent {
	return domain.SessionEvent{SessionID: session, Kind: domain.EventPreToolUse, Tool: tool, Detail: detail}
}

func cardFixture() rpc.State {
	st := fixture(5, 1)
	for _, cmd := range []string{"go build", "go vet", "go test ./...", "git status"} {
		st.Events = append(st.Events, toolEvent("s05", "Bash", cmd))
	}
	st.Events = append(st.Events, domain.SessionEvent{
		SessionID: "s05", Kind: domain.EventPermissionRequest,
		Text: "Bash: rm -rf build\nmake clean\nmake all\nmake install\nmake deploy",
	})
	return st
}

func selectSession(t *testing.T, m tui.Model, id string) tui.Model {
	t.Helper()
	for i := range 9 {
		if m.Selected() == id {
			return m
		}
		m = press(m, fmt.Sprint(i+1))
	}
	if m.Selected() != id {
		t.Fatalf("could not select %s", id)
	}
	return m
}

func TestSessionCardShowsTaskPRsActionsAndWhatItWaitsOn(t *testing.T) {
	st := cardFixture()
	m := selectSession(t, newModel(&st, nil), "s05")
	out := screen(m)
	card := out[strings.Index(out, "CARD"):]
	for _, want := range []string{
		"#42 task number 3", "#3604",
		"git status", "go test ./...", "go vet",
		"Bash: rm -rf build", "make clean", "make all…",
	} {
		if !strings.Contains(card, want) {
			t.Errorf("card missing %q:\n%s", want, card)
		}
	}
	for _, absent := range []string{"go build", "make install", "make deploy"} {
		if strings.Contains(card, absent) {
			t.Errorf("card has %q:\n%s", absent, card)
		}
	}
	if strings.Index(card, "git status") > strings.Index(card, "go vet") {
		t.Errorf("actions are not newest first:\n%s", card)
	}
}

func TestSessionCardFollowsTheSelectionAndLiveEvents(t *testing.T) {
	st := fixture(2, 1)
	st.Events = []domain.SessionEvent{toolEvent("s01", "Read", "a.go"), toolEvent("s02", "Read", "b.go")}
	m := newModel(&st, nil)
	if out := screen(m); !strings.Contains(out, "Read a.go") || strings.Contains(out, "Read b.go") {
		t.Fatalf("first session's card:\n%s", out)
	}
	m = press(m, "j")
	if out := screen(m); !strings.Contains(out, "Read b.go") || strings.Contains(out, "Read a.go") {
		t.Fatalf("second session's card:\n%s", out)
	}
	s := st.Sessions[1]
	ev := toolEvent("s02", "Bash", "make")
	m = update(m, tui.DiffMsg(rpc.Diff{Seq: 1, Session: &s, Event: &ev}))
	if out := screen(m); !strings.Contains(out, "last  make") || !strings.Contains(out, "Read b.go") {
		t.Fatalf("card after a live event:\n%s", out)
	}
}

func TestNoCardWithoutSessions(t *testing.T) {
	if out := screen(newModel(nil, nil)); strings.Contains(out, "CARD") {
		t.Fatalf("empty sidebar has a card:\n%s", out)
	}
}

func TestGoldenSessionCard(t *testing.T) {
	st := cardFixture()
	m := selectSession(t, newModel(&st, nil), "s05")
	golden.RequireEqual(t, screen(m))
}

func TestTheFooterLeavesTheSessionListItsRows(t *testing.T) {
	st := cardFixture()
	out := screen(selectSession(t, newModel(&st, nil), "s05"))
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, " 5 ") && i+1 < len(lines) && strings.Contains(lines[i+1], "opus-5.5") {
			return
		}
	}
	t.Fatalf("session 5's model line is cut off:\n%s", out)
}

package domain

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSubagentFromHookReadsTheAgentIdentity(t *testing.T) {
	start, ok := SubagentFromHook(EventSubagentStart, t0, []byte(`{"agent_id":"a1","agent_type":"Explore"}`))
	if !ok || start.ID != "a1" || start.Type != "Explore" || start.State != SubagentRunning || !start.StartedAt.Equal(t0) {
		t.Fatalf("start %+v, %v", start, ok)
	}
	stop, ok := SubagentFromHook(EventSubagentStop, t0, []byte(`{"agent_id":"a1","agent_type":"Explore","last_assistant_message":"Found 3 callers.\nDetails follow."}`))
	if !ok || stop.State != SubagentStopped || !stop.StoppedAt.Equal(t0) || stop.Summary != "Found 3 callers." {
		t.Fatalf("stop %+v, %v", stop, ok)
	}
}

func TestSubagentFromHookNeedsAnAgentID(t *testing.T) {
	for _, payload := range []string{``, `not json`, `{}`, `{"agent_type":"Explore"}`} {
		if _, ok := SubagentFromHook(EventSubagentStart, t0, []byte(payload)); ok {
			t.Errorf("payload %q gave a subagent", payload)
		}
	}
	if _, ok := SubagentFromHook(EventPreToolUse, t0, []byte(`{"agent_id":"a1"}`)); ok {
		t.Error("a tool event gave a subagent")
	}
}

func TestSubagentSummaryIsCutToOneShortLine(t *testing.T) {
	long := strings.Repeat("x", 300)
	stop, _ := SubagentFromHook(EventSubagentStop, t0, []byte(`{"agent_id":"a1","last_assistant_message":"`+long+`"}`))
	if got := len([]rune(stop.Summary)); got != MaxDetail {
		t.Fatalf("summary is %d runes", got)
	}
}

func sub(session, id string, kind HarnessEventKind, at time.Time) Subagent {
	s, _ := SubagentFromHook(kind, at, []byte(fmt.Sprintf(`{"agent_id":%q,"agent_type":"Explore"}`, id)))
	s.SessionID = session
	return s
}

func TestTrackSubagentStartsThenStopsTheSameAgent(t *testing.T) {
	subs, changed := TrackSubagent(nil, sub("s", "a1", EventSubagentStart, t0))
	if len(subs) != 1 || changed.State != SubagentRunning {
		t.Fatalf("after start %+v", subs)
	}
	later := t0.Add(time.Minute)
	subs, changed = TrackSubagent(subs, sub("s", "a1", EventSubagentStop, later))
	if len(subs) != 1 || changed.State != SubagentStopped || !changed.StartedAt.Equal(t0) || !changed.StoppedAt.Equal(later) || changed.Type != "Explore" {
		t.Fatalf("after stop %+v, changed %+v", subs, changed)
	}
}

func TestTrackSubagentKeepsTheTypeWhenStopCarriesNone(t *testing.T) {
	subs, _ := TrackSubagent(nil, sub("s", "a1", EventSubagentStart, t0))
	stop := Subagent{SessionID: "s", ID: "a1", State: SubagentStopped, StoppedAt: t0}
	_, changed := TrackSubagent(subs, stop)
	if changed.Type != "Explore" {
		t.Fatalf("type %q", changed.Type)
	}
}

func TestTrackSubagentAddsAStopSeenWithoutItsStart(t *testing.T) {
	subs, changed := TrackSubagent(nil, sub("s", "a1", EventSubagentStop, t0))
	if len(subs) != 1 || changed.State != SubagentStopped || !changed.StartedAt.Equal(t0) {
		t.Fatalf("got %+v", subs)
	}
}

func TestTrackSubagentSeparatesSessionsThatReuseAnAgentID(t *testing.T) {
	subs, _ := TrackSubagent(nil, sub("s1", "a1", EventSubagentStart, t0))
	subs, _ = TrackSubagent(subs, sub("s2", "a1", EventSubagentStart, t0))
	subs, _ = TrackSubagent(subs, sub("s1", "a1", EventSubagentStop, t0))
	if len(subs) != 2 || subs[0].State != SubagentStopped || subs[1].State != SubagentRunning {
		t.Fatalf("got %+v", subs)
	}
}

func TestTrackSubagentDropsTheOldestStoppedPastTheCap(t *testing.T) {
	var subs []Subagent
	for i := range MaxSubagents + 5 {
		id := fmt.Sprintf("a%02d", i)
		subs, _ = TrackSubagent(subs, sub("s", id, EventSubagentStart, t0))
		subs, _ = TrackSubagent(subs, sub("s", id, EventSubagentStop, t0))
	}
	subs, _ = TrackSubagent(subs, sub("other", "x", EventSubagentStart, t0))
	if len(subs) != MaxSubagents+1 {
		t.Fatalf("kept %d", len(subs))
	}
	if subs[0].ID != "a05" {
		t.Fatalf("oldest kept is %s", subs[0].ID)
	}
}

func TestTrackSubagentNeverDropsARunningOne(t *testing.T) {
	subs, _ := TrackSubagent(nil, sub("s", "running", EventSubagentStart, t0))
	for i := range MaxSubagents + 2 {
		id := fmt.Sprintf("a%02d", i)
		subs, _ = TrackSubagent(subs, sub("s", id, EventSubagentStart, t0))
		subs, _ = TrackSubagent(subs, sub("s", id, EventSubagentStop, t0))
	}
	if subs[0].ID != "running" || subs[0].State != SubagentRunning {
		t.Fatalf("first is %+v", subs[0])
	}
}

func TestEndSubagentsStopsOnlyThatSessionsRunningAgents(t *testing.T) {
	var subs []Subagent
	subs, _ = TrackSubagent(subs, sub("s1", "a1", EventSubagentStart, t0))
	subs, _ = TrackSubagent(subs, sub("s1", "a2", EventSubagentStart, t0))
	subs, _ = TrackSubagent(subs, sub("s1", "a2", EventSubagentStop, t0))
	subs, _ = TrackSubagent(subs, sub("s2", "b1", EventSubagentStart, t0))
	end := t0.Add(time.Hour)
	subs, changed := EndSubagents(subs, "s1", end)
	if len(changed) != 1 || changed[0].ID != "a1" || !changed[0].StoppedAt.Equal(end) {
		t.Fatalf("changed %+v", changed)
	}
	if subs[0].State != SubagentStopped || subs[2].State != SubagentRunning || !subs[1].StoppedAt.Equal(t0) {
		t.Fatalf("subs %+v", subs)
	}
}

func TestSubagentTreeNestsChildrenUnderTheirParent(t *testing.T) {
	subs := []Subagent{
		{ID: "a"}, {ID: "b", ParentID: "a"}, {ID: "c"}, {ID: "d", ParentID: "b"}, {ID: "e", ParentID: "gone"},
	}
	var got []string
	for _, n := range SubagentTree(subs) {
		got = append(got, fmt.Sprintf("%d%s", n.Depth, n.ID))
	}
	want := []string{"0a", "1b", "2d", "0c", "0e"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSubagentTreeSurvivesAParentCycle(t *testing.T) {
	subs := []Subagent{{ID: "a", ParentID: "b"}, {ID: "b", ParentID: "a"}}
	if got := SubagentTree(subs); len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
}

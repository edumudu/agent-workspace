package domain

import (
	"reflect"
	"testing"
)

func sessionIDs(groups []TaskGroup) [][]string {
	out := [][]string{}
	for _, g := range groups {
		ids := []string{}
		for _, s := range g.Sessions {
			ids = append(ids, s.ID)
		}
		out = append(out, ids)
	}
	return out
}

func TestSidebarGroupsSessionsUnderTheirTaskInTaskOrder(t *testing.T) {
	tasks := []Task{{ID: "t1"}, {ID: "t2"}, {ID: "empty"}}
	sessions := []Session{
		{ID: "b", TaskID: "t2"},
		{ID: "a", TaskID: "t1"},
		{ID: "c", TaskID: "t1"},
	}
	groups := Sidebar(tasks, sessions)
	if got, want := sessionIDs(groups), [][]string{{"a", "c"}, {"b"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
	if groups[0].Task.ID != "t1" || groups[1].Task.ID != "t2" {
		t.Fatalf("task order = %s, %s", groups[0].Task.ID, groups[1].Task.ID)
	}
}

func TestSidebarPutsSessionsThatNeedYouFirstInTheirGroup(t *testing.T) {
	tests := []struct {
		name   string
		states []AgentState
		want   []string
	}{
		{"waiting before running", []AgentState{StateRunning, StateWaiting}, []string{"s1", "s0"}},
		{"permission before idle", []AgentState{StateIdle, StatePermission}, []string{"s1", "s0"}},
		{"done does not need you", []AgentState{StateRunning, StateDone}, []string{"s0", "s1"}},
		{"ties keep their order", []AgentState{StateWaiting, StatePermission, StateIdle}, []string{"s0", "s1", "s2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sessions []Session
			for i, st := range tt.states {
				sessions = append(sessions, Session{ID: "s" + string(rune('0'+i)), TaskID: "t", State: st})
			}
			got := sessionIDs(Sidebar([]Task{{ID: "t"}}, sessions))
			if !reflect.DeepEqual(got, [][]string{tt.want}) {
				t.Fatalf("order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionsWithAnUnknownTaskGetTheirOwnGroupAtTheEnd(t *testing.T) {
	groups := Sidebar([]Task{{ID: "t1"}}, []Session{{ID: "x", TaskID: "gone"}, {ID: "a", TaskID: "t1"}, {ID: "y", TaskID: "gone"}})
	if got, want := sessionIDs(groups), [][]string{{"a"}, {"x", "y"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
	if groups[1].Task.ID != "gone" {
		t.Fatalf("orphan group task = %q", groups[1].Task.ID)
	}
}

func TestNeedsYouCountsWaitingAndPermission(t *testing.T) {
	tests := map[AgentState]bool{
		StateWaiting: true, StatePermission: true,
		StateRunning: false, StateIdle: false, StateDone: false,
	}
	for st, want := range tests {
		if got := (Session{State: st}).NeedsYou(); got != want {
			t.Errorf("%s NeedsYou = %v, want %v", st, got, want)
		}
	}
}

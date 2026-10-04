package domain

import (
	"testing"
	"time"
)

func TestStateSinceIsWhenTheReplayedEventsLastChangedTheState(t *testing.T) {
	at := func(m int) time.Time { return bannerT0.Add(time.Duration(m) * time.Minute) }
	ev := func(m int, kind HarnessEventKind) SessionEvent { return SessionEvent{SessionID: "a", At: at(m), Kind: kind} }
	cases := []struct {
		name   string
		state  AgentState
		events []SessionEvent
		want   time.Time
		ok     bool
	}{
		{"asking for permission", StatePermission,
			[]SessionEvent{ev(0, EventUserPromptSubmit), ev(1, EventPreToolUse), ev(2, EventPermissionRequest)}, at(2), true},
		{"tools keep a turn running since its prompt", StateRunning,
			[]SessionEvent{ev(0, EventUserPromptSubmit), ev(1, EventPreToolUse), ev(2, EventPostToolUse)}, at(0), true},
		{"running again once permission is granted", StateRunning,
			[]SessionEvent{ev(0, EventUserPromptSubmit), ev(1, EventPermissionRequest), ev(3, EventPostToolUse)}, at(3), true},
		{"a second stop does not restart done", StateDone,
			[]SessionEvent{ev(0, EventUserPromptSubmit), ev(4, EventStop), ev(6, EventStop)}, at(4), true},
		{"idle since the session started", StateIdle,
			[]SessionEvent{ev(5, EventSessionStart)}, at(5), true},
		{"a window that starts mid-turn", StateWaiting,
			[]SessionEvent{ev(7, EventWaitingForInput)}, at(7), true},
		{"other sessions' events do not count", StateRunning,
			[]SessionEvent{ev(0, EventUserPromptSubmit), {SessionID: "b", At: at(9), Kind: EventStop}}, at(0), true},
		{"no events", StateRunning, nil, time.Time{}, false},
		{"events that disagree with the state", StateDone,
			[]SessionEvent{ev(0, EventUserPromptSubmit)}, time.Time{}, false},
	}
	for _, tt := range cases {
		got, ok := StateSince(Session{ID: "a", State: tt.state}, tt.events)
		if ok != tt.ok || !got.Equal(tt.want) {
			t.Errorf("%s: got %v, %v; want %v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestWorktreeLabelNamesTheFirstRepoAndBranch(t *testing.T) {
	cases := []struct {
		name      string
		worktrees []Worktree
		want      string
	}{
		{"none", nil, ""},
		{"repo and branch", []Worktree{{Repo: "api", Branch: "login"}}, "api@login"},
		{"a repo path shows its last element", []Worktree{{Repo: "/Users/me/src/api/", Branch: "main"}}, "api@main"},
		{"no branch", []Worktree{{Repo: "api"}}, "api"},
		{"several count the rest", []Worktree{{Repo: "api", Branch: "a"}, {Repo: "web", Branch: "b"}, {Repo: "cli", Branch: "c"}}, "api@a +2"},
		{"a long branch is cut", []Worktree{{Repo: "api", Branch: "feature/a-very-long-branch-name-that-goes-on"}}, "api@feature/a-very-long-branch-n…"},
	}
	for _, tt := range cases {
		if got := WorktreeLabel(tt.worktrees); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

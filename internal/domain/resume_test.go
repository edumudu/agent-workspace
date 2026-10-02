package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestResumeIDFromHook(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"claude and codex hooks", `{"session_id":"4c98","cwd":"/w"}`, "4c98"},
		{"codex notify", `{"type":"agent-turn-complete","thread-id":"t-1"}`, "t-1"},
		{"no id", `{"cwd":"/w"}`, ""},
		{"not json", `nope`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResumeIDFromHook([]byte(c.payload)); got != c.want {
				t.Fatalf("ResumeIDFromHook = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSessionIsResumableOnceEndedWithAnIDAndADir(t *testing.T) {
	cases := []struct {
		name string
		s    Session
		want bool
	}{
		{"ended with id and dir", Session{Ended: true, ResumeID: "r", Dir: "/w"}, true},
		{"live", Session{Pane: "%1", ResumeID: "r", Dir: "/w"}, false},
		{"ended with no id", Session{Ended: true, Dir: "/w"}, false},
		{"ended with no dir", Session{Ended: true, ResumeID: "r"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.Resumable(); got != c.want {
				t.Fatalf("Resumable() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestResumedSessionIsLiveOnItsNewPaneAndKeepsItsWork(t *testing.T) {
	s := Session{ID: "a", TaskID: "t", Ended: true, State: StateIdle, ResumeID: "r", Dir: "/w", Model: "opus", Effort: "high", WorktreeIDs: []string{"w"}, Unread: true}
	got := s.Resumed("%9")
	want := Session{ID: "a", TaskID: "t", Pane: "%9", State: StateIdle, ResumeID: "r", Dir: "/w", Model: "opus", Effort: "high", WorktreeIDs: []string{"w"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resumed() = %+v, want %+v", got, want)
	}
}

func TestResumableSessionsListsTheMostRecentlyActiveFirst(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	old := Session{ID: "old", Ended: true, ResumeID: "r1", Dir: "/w"}
	recent := Session{ID: "recent", Ended: true, ResumeID: "r2", Dir: "/w"}
	quiet := Session{ID: "quiet", Ended: true, ResumeID: "r3", Dir: "/w"}
	live := Session{ID: "live", Pane: "%1", ResumeID: "r4", Dir: "/w"}
	events := map[string][]SessionEvent{
		"old":    {{At: at}, {At: at.Add(time.Minute)}},
		"recent": {{At: at.Add(time.Hour)}},
		"live":   {{At: at.Add(2 * time.Hour)}},
	}
	got := ResumableSessions([]Session{quiet, old, live, recent}, events)
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if want := []string{"recent", "old", "quiet"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ResumableSessions = %v, want %v", ids, want)
	}
}

func TestRemovingTheWorktreeASessionRanInMakesItUnresumable(t *testing.T) {
	s := Session{Ended: true, ResumeID: "r", Dir: "/w/a", WorktreeIDs: []string{"/w/a", "/w/b"}}
	if got := s.DetachWorktree("/w/b"); got.Dir != "/w/a" || !got.Resumable() {
		t.Fatalf("another worktree removed: %+v", got)
	}
	if got := s.DetachWorktree("/w/a"); got.Dir != "" || got.Resumable() {
		t.Fatalf("its own worktree removed: %+v", got)
	}
}

package domain

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNotifyBannerFor(t *testing.T) {
	cases := []struct {
		name    string
		session Session
		title   string
		effect  Effect
		want    Banner
		ok      bool
	}{
		{"permission", Session{Harness: HarnessClaude}, "fix login", notify(StatePermission), Banner{Title: "fix login", Body: "needs permission", State: StatePermission}, true},
		{"waiting", Session{Harness: HarnessCodex}, "fix login", notify(StateWaiting), Banner{Title: "fix login", Body: "waiting", State: StateWaiting}, true},
		{"done", Session{Harness: HarnessClaude}, "fix login", notify(StateDone), Banner{Title: "fix login", Body: "done", State: StateDone}, true},
		{"muted", Session{Muted: true}, "fix login", notify(StateDone), Banner{}, false},
		{"not a notify effect", Session{}, "fix login", markUnread, Banner{}, false},
		{"unnamed falls back to the harness", Session{Harness: HarnessCodex}, "  ", notify(StateDone), Banner{Title: "codex", Body: "done", State: StateDone}, true},
	}
	for _, tt := range cases {
		got, ok := BannerFor(BannerInput{Session: tt.session, Name: tt.title, Effect: tt.effect})
		if ok != tt.ok || got != tt.want {
			t.Errorf("%s: got %+v, %v; want %+v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

var bannerT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func bannerTurn(sessionID string, evs ...SessionEvent) []SessionEvent {
	out := []SessionEvent{{SessionID: sessionID, At: bannerT0, Kind: EventUserPromptSubmit}}
	for _, ev := range evs {
		ev.SessionID = sessionID
		if ev.At.IsZero() {
			ev.At = bannerT0
		}
		out = append(out, ev)
	}
	return out
}

func TestNotifyBannerContent(t *testing.T) {
	api := Worktree{ID: "w1", Repo: "api", Branch: "131-notifications"}
	web := Worktree{ID: "w2", Repo: "web", Branch: "main"}
	claude := Session{ID: "a", Harness: HarnessClaude}
	cases := []struct {
		name      string
		in        BannerInput
		wantTitle string
		wantBody  string
	}{
		{"title has repo and branch",
			BannerInput{Session: claude, Name: "fix login", Effect: notify(StateDone), Worktrees: []Worktree{api}},
			"fix login · api@131-notifications", "done"},
		{"repo given as a path shows its last element",
			BannerInput{Session: claude, Name: "fix login", Effect: notify(StateDone), Worktrees: []Worktree{{Repo: "/Users/me/src/api/", Branch: "main"}}},
			"fix login · api@main", "done"},
		{"several worktrees name the first and count the rest",
			BannerInput{Session: claude, Name: "fix login", Effect: notify(StateDone), Worktrees: []Worktree{api, web}},
			"fix login · api@131-notifications +1", "done"},
		{"long branch is cut",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateDone), Worktrees: []Worktree{{Repo: "api", Branch: "feature/a-very-long-branch-name-that-goes-on"}}},
			"x · api@feature/a-very-long-branch-n…", "done"},
		{"done says the first line of the last message and the elapsed time",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateDone), Now: bannerT0.Add(4*time.Minute + 12*time.Second),
				Events: bannerTurn("a", SessionEvent{Kind: EventStop, Text: "\n  Added the retry to the client.\n\nDetails follow."})},
			"x", "Added the retry to the client. (4m12s)"},
		{"done with no message says how long it took",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateDone), Now: bannerT0.Add(90 * time.Minute),
				Events: bannerTurn("a", SessionEvent{Kind: EventStop})},
			"x", "done in 1h30m"},
		{"done ignores another session's events",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateDone), Now: bannerT0.Add(time.Minute),
				Events: bannerTurn("b", SessionEvent{Kind: EventStop, Text: "not mine"})},
			"x", "done"},
		{"permission names the tool and target",
			BannerInput{Session: claude, Name: "x", Effect: notify(StatePermission),
				Events: bannerTurn("a", SessionEvent{Kind: EventPermissionRequest, Tool: "Bash", Detail: "npm test"})},
			"x", "needs permission: Bash: npm test"},
		{"permission without a tool uses the message",
			BannerInput{Session: claude, Name: "x", Effect: notify(StatePermission),
				Events: bannerTurn("a", SessionEvent{Kind: EventPermissionRequest, Text: "Claude needs your permission to use Edit"})},
			"x", "needs permission: Claude needs your permission to use Edit"},
		{"waiting quotes the question",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateWaiting),
				Events: bannerTurn("a", SessionEvent{Kind: EventStop, Text: "I found two options.\n\nShould I use the cache?"}, SessionEvent{Kind: EventWaitingForInput, Text: "Claude is waiting for your input"})},
			"x", "asks: Should I use the cache?"},
		{"waiting without a question uses the notification",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateWaiting),
				Events: bannerTurn("a", SessionEvent{Kind: EventWaitingForInput, Text: "Claude is waiting for your input"})},
			"x", "waiting: Claude is waiting for your input"},
		{"limit hit is explicit",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateDone), Now: bannerT0.Add(time.Minute),
				Events: bannerTurn("a", SessionEvent{Kind: EventStop, Text: "You've hit your usage limit · resets 5pm"})},
			"x", "usage limit: You've hit your usage limit · resets 5pm"},
		{"api error is explicit",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateDone), Now: bannerT0.Add(time.Minute),
				Events: bannerTurn("a", SessionEvent{Kind: EventStop, Text: "API Error: 529 overloaded"})},
			"x", "error: API Error: 529 overloaded"},
		{"secrets are masked",
			BannerInput{Session: claude, Name: "x", Effect: notify(StatePermission),
				Events: bannerTurn("a", SessionEvent{Kind: EventPermissionRequest, Tool: "Bash", Detail: "curl -H token=abc123 https://x"})},
			"x", "needs permission: Bash: curl -H token=… https://x"},
		{"long body is truncated",
			BannerInput{Session: claude, Name: "x", Effect: notify(StateDone),
				Events: bannerTurn("a", SessionEvent{Kind: EventStop, Text: strings.Repeat("word ", 60)})},
			"x", strings.Repeat("word ", 23) + "word…"},
	}
	for _, tt := range cases {
		got, ok := BannerFor(tt.in)
		if !ok || got.Title != tt.wantTitle || got.Body != tt.wantBody {
			t.Errorf("%s: got %q / %q, %v; want %q / %q", tt.name, got.Title, got.Body, ok, tt.wantTitle, tt.wantBody)
		}
	}
}

func TestNotifyBannerMasksSecretTokens(t *testing.T) {
	for _, in := range []string{
		"export GITHUB_TOKEN=ghp_abcdefghijklmnop",
		"use sk-ant-api03-abcdefghijklmnopqrstuvwxyz",
		"password: hunter2",
		"API_KEY=abcd",
	} {
		b, _ := BannerFor(BannerInput{Session: Session{ID: "a"}, Name: "x", Effect: notify(StatePermission),
			Events: bannerTurn("a", SessionEvent{Kind: EventPermissionRequest, Tool: "Bash", Detail: in})})
		for _, secret := range []string{"ghp_abc", "sk-ant", "hunter2", "abcd"} {
			if strings.Contains(b.Body, secret) {
				t.Errorf("%q leaked into %q", in, b.Body)
			}
		}
	}
}

func TestNotifyBannerExamplesGolden(t *testing.T) {
	api := Worktree{Repo: "api", Branch: "42-retry-client"}
	web := Worktree{Repo: "web", Branch: "fix/login-redirect"}
	examples := []BannerInput{
		{Session: Session{ID: "a", Harness: HarnessClaude}, Name: "retry the client", Effect: notify(StateDone), Worktrees: []Worktree{api}, Now: bannerT0.Add(6*time.Minute + 3*time.Second),
			Events: bannerTurn("a", SessionEvent{Kind: EventStop, Text: "Added exponential backoff to the HTTP client and covered it with tests.\n\nAll 48 tests pass."})},
		{Session: Session{ID: "a", Harness: HarnessClaude}, Name: "retry the client", Effect: notify(StatePermission), Worktrees: []Worktree{api},
			Events: bannerTurn("a", SessionEvent{Kind: EventPermissionRequest, Tool: "Bash", Detail: "go test ./internal/client/..."})},
		{Session: Session{ID: "b", Harness: HarnessCodex}, Name: "login redirect", Effect: notify(StatePermission), Worktrees: []Worktree{web},
			Events: bannerTurn("b", SessionEvent{Kind: EventPermissionRequest, Tool: "Edit", Detail: "/src/routes/login.ts"})},
		{Session: Session{ID: "b", Harness: HarnessClaude}, Name: "login redirect", Effect: notify(StateWaiting), Worktrees: []Worktree{web},
			Events: bannerTurn("b", SessionEvent{Kind: EventStop, Text: "The redirect loses the query string.\n\nShould I keep the old /signin route as an alias?"})},
		{Session: Session{ID: "c", Harness: HarnessClaude}, Name: "", Effect: notify(StateDone), Worktrees: []Worktree{api, web}, Now: bannerT0.Add(2 * time.Hour),
			Events: bannerTurn("c", SessionEvent{Kind: EventStop, Text: "You've hit your usage limit · resets 5pm"})},
	}
	var sb strings.Builder
	for _, in := range examples {
		b, _ := BannerFor(in)
		fmt.Fprintf(&sb, "[%s]\n%s\n%s\n\n", b.State, b.Title, b.Body)
	}
	want, err := os.ReadFile("testdata/banners.golden")
	if err != nil {
		t.Fatal(err)
	}
	if sb.String() != string(want) {
		t.Fatalf("banners differ from testdata/banners.golden; got:\n%s", sb.String())
	}
}

func TestNotifyMutedSessionStillGoesUnread(t *testing.T) {
	s := Session{State: StateRunning, Muted: true}
	next, effects := s.Apply(HarnessEvent{Kind: EventStop})
	if !next.Unread {
		t.Fatal("muted session is not unread after done")
	}
	for _, e := range effects {
		if _, ok := BannerFor(BannerInput{Session: next, Name: "x", Effect: e}); ok {
			t.Fatalf("muted session produced a banner for %+v", e)
		}
	}
}

func TestNotifySetMutedOnlyChangesMuted(t *testing.T) {
	s := Session{ID: "a", State: StateWaiting, Unread: true}
	got := s.SetMuted(true)
	if got.ID != "a" || got.State != StateWaiting || !got.Unread || !got.Muted {
		t.Fatalf("got %+v", got)
	}
	if got.SetMuted(false).Muted {
		t.Fatal("unmute kept Muted")
	}
}

func TestNotifyCoalescerAllowsOneBannerPerSessionPerWindow(t *testing.T) {
	c := NewCoalescer()
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	steps := []struct {
		session string
		at      time.Duration
		want    bool
	}{
		{"a", 0, true},
		{"a", time.Second, false},
		{"a", CoalesceWindow - time.Nanosecond, false},
		{"b", time.Second, true},
		{"a", CoalesceWindow, true},
		{"a", CoalesceWindow + time.Second, false},
	}
	for i, st := range steps {
		if got := c.Allow(st.session, t0.Add(st.at)); got != st.want {
			t.Fatalf("step %d (%s at %v): got %v", i, st.session, st.at, got)
		}
	}
}

func TestNotifyBurstOfTwentyAttentionEventsIsOneBanner(t *testing.T) {
	c := NewCoalescer()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s := Session{ID: "a", State: StateRunning}
	banners := 0
	for i := range 20 {
		var effects []Effect
		s, effects = s.Apply(HarnessEvent{Kind: EventPermissionRequest})
		for _, e := range effects {
			if _, ok := BannerFor(BannerInput{Session: s, Name: "x", Effect: e}); ok && c.Allow(s.ID, now.Add(time.Duration(i)*100*time.Millisecond)) {
				banners++
			}
		}
		s, _ = s.Apply(HarnessEvent{Kind: EventPostToolUse})
	}
	if banners != 1 {
		t.Fatalf("banners = %d, want 1", banners)
	}
}

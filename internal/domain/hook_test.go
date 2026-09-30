package domain

import "testing"

func TestHookEventNamesMapToHarnessEvents(t *testing.T) {
	cases := []struct {
		harness Harness
		name    string
		want    HarnessEventKind
		ok      bool
	}{
		{HarnessClaude, "SessionStart", EventSessionStart, true},
		{HarnessClaude, "UserPromptSubmit", EventUserPromptSubmit, true},
		{HarnessClaude, "PreToolUse", EventPreToolUse, true},
		{HarnessClaude, "PostToolUse", EventPostToolUse, true},
		{HarnessClaude, "PermissionRequest", EventPermissionRequest, true},
		{HarnessClaude, "Notification", EventWaitingForInput, true},
		{HarnessClaude, "Stop", EventStop, true},
		{HarnessClaude, "SessionEnd", EventSessionEnd, true},
		{HarnessClaude, "SubagentStop", "", false},
		{HarnessCodex, "SessionStart", EventSessionStart, true},
		{HarnessCodex, "UserPromptSubmit", EventUserPromptSubmit, true},
		{HarnessCodex, "PreToolUse", EventPreToolUse, true},
		{HarnessCodex, "PostToolUse", EventPostToolUse, true},
		{HarnessCodex, "Stop", EventStop, true},
		{HarnessCodex, "PermissionRequest", EventPermissionRequest, true},
		{HarnessCodex, "Interrupt", EventStop, true},
		{HarnessCodex, "SessionEnd", EventSessionEnd, true},
		{HarnessCodex, "SubagentStop", "", false},
		{HarnessCodex, "PreCompact", "", false},
		{HarnessCodex, "Notification", "", false},
		{"other", "Stop", "", false},
	}
	for _, c := range cases {
		got, ok := HookEvent(c.harness, c.name)
		if got != c.want || ok != c.ok {
			t.Errorf("HookEvent(%q, %q) = %q, %v; want %q, %v", c.harness, c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestOnlyUserPromptSubmitExpectsAReply(t *testing.T) {
	for _, name := range []string{"SessionStart", "PreToolUse", "PostToolUse", "Stop", "Notification", "PermissionRequest"} {
		if HookWantsReply(name) {
			t.Errorf("%s wants a reply", name)
		}
	}
	if !HookWantsReply("UserPromptSubmit") {
		t.Error("UserPromptSubmit wants no reply")
	}
}

func TestSessionOnPaneFindsOnlyThatPane(t *testing.T) {
	sessions := []Session{{ID: "a", Pane: "%1"}, {ID: "b", Pane: "%2"}}
	if s, ok := SessionOnPane(sessions, "%2"); !ok || s.ID != "b" {
		t.Fatalf("got %+v, %v", s, ok)
	}
	for _, pane := range []string{"%9", ""} {
		if _, ok := SessionOnPane(append(sessions, Session{ID: "c"}), pane); ok {
			t.Fatalf("pane %q matched", pane)
		}
	}
}

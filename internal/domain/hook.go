package domain

var claudeHooks = map[string]HarnessEventKind{
	"SessionStart":      EventSessionStart,
	"UserPromptSubmit":  EventUserPromptSubmit,
	"PreToolUse":        EventPreToolUse,
	"PostToolUse":       EventPostToolUse,
	"PermissionRequest": EventPermissionRequest,
	"Notification":      EventWaitingForInput,
	"Stop":              EventStop,
	"SubagentStart":     EventSubagentStart,
	"SubagentStop":      EventSubagentStop,
	"SessionEnd":        EventSessionEnd,
}

// ClaudeNotification refines a Claude Notification hook by its
// notification_type: a permission prompt needs approval, anything else that
// waits on the user is waiting. Types that ask nothing of the user report false.
func ClaudeNotification(notificationType string) (HarnessEventKind, bool) {
	switch notificationType {
	case "permission_prompt":
		return EventPermissionRequest, true
	case "auth_success":
		return "", false
	}
	return EventWaitingForInput, true
}

var codexHooks = map[string]HarnessEventKind{
	"SessionStart":     EventSessionStart,
	"UserPromptSubmit": EventUserPromptSubmit,
	"PreToolUse":       EventPreToolUse,
	"PostToolUse":      EventPostToolUse,
	"Stop":             EventStop,
	// why: Codex fires Interrupt instead of Stop when the user aborts a turn.
	"Interrupt":         EventStop,
	"PermissionRequest": EventPermissionRequest,
	"SessionEnd":        EventSessionEnd,
}

// HookEvent translates a harness hook name, as passed to `agentws hook
// --event`, into the harness-neutral event. Unknown names report false.
func HookEvent(h Harness, name string) (HarnessEventKind, bool) {
	var table map[string]HarnessEventKind
	switch h {
	case HarnessClaude:
		table = claudeHooks
	case HarnessCodex:
		table = codexHooks
	}
	kind, ok := table[name]
	return kind, ok
}

// HookWantsReply reports whether the harness reads the hook's stdout, so the
// hook must ask the daemon for a reply instead of fire-and-forget.
func HookWantsReply(name string) bool {
	return name == "UserPromptSubmit"
}

// SessionOnPane finds the session running in a tmux pane. An empty pane
// matches nothing.
func SessionOnPane(sessions []Session, pane string) (Session, bool) {
	if pane == "" {
		return Session{}, false
	}
	for _, s := range sessions {
		if s.Pane == pane {
			return s, true
		}
	}
	return Session{}, false
}

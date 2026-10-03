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
	"SessionStart":      EventSessionStart,
	"UserPromptSubmit":  EventUserPromptSubmit,
	"PreToolUse":        EventPreToolUse,
	"PostToolUse":       EventPostToolUse,
	"Stop":              EventStop,
	"Interrupt":         EventStop,
	"PermissionRequest": EventPermissionRequest,
	"SessionEnd":        EventSessionEnd,
}

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

func HookWantsReply(name string) bool {
	return name == "UserPromptSubmit"
}

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

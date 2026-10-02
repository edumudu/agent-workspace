package domain

type HarnessSpec struct {
	Harness Harness
	Tag     string
	Hooks   map[string]HarnessEventKind
}

var harnessTable = []HarnessSpec{
	{
		Harness: HarnessClaude,
		Tag:     "CC",
		Hooks: map[string]HarnessEventKind{
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
		},
	},
	{
		Harness: HarnessCodex,
		Tag:     "CX",
		Hooks: map[string]HarnessEventKind{
			"SessionStart":     EventSessionStart,
			"UserPromptSubmit": EventUserPromptSubmit,
			"PreToolUse":       EventPreToolUse,
			"PostToolUse":      EventPostToolUse,
			"Stop":             EventStop,
			// why: Codex fires Interrupt instead of Stop when the user aborts a turn.
			"Interrupt":         EventStop,
			"PermissionRequest": EventPermissionRequest,
			"SessionEnd":        EventSessionEnd,
		},
	},
	{
		Harness: HarnessOmp,
		Tag:     "OM",
		// why: agent_end carries willContinue and turn_end is one model round, so
		// neither means done; session_switch would idle the pane on a fork. An
		// approval is not an event: the tool_call after it sets running.
		Hooks: map[string]HarnessEventKind{
			"session_start":           EventSessionStart,
			"agent_start":             EventUserPromptSubmit,
			"tool_call":               EventPreToolUse,
			"tool_result":             EventPostToolUse,
			"tool_approval_requested": EventPermissionRequest,
			"tool_approval_resolved":  EventWaitingForInput,
			"session_stop":            EventStop,
			"session_shutdown":        EventSessionEnd,
		},
	},
}

func Harnesses() []Harness {
	out := make([]Harness, len(harnessTable))
	for i, spec := range harnessTable {
		out[i] = spec.Harness
	}
	return out
}

func Spec(h Harness) HarnessSpec {
	for _, spec := range harnessTable {
		if spec.Harness == h {
			return spec
		}
	}
	return HarnessSpec{Harness: h}
}

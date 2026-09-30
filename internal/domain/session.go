package domain

type AgentState string

const (
	StateIdle       AgentState = "idle"
	StateRunning    AgentState = "running"
	StateWaiting    AgentState = "waiting"
	StatePermission AgentState = "permission"
	StateDone       AgentState = "done"
)

// HarnessEventKind is the harness-neutral event the Claude and Codex adapters
// translate their hook payloads into.
type HarnessEventKind string

const (
	EventSessionStart      HarnessEventKind = "session_start"
	EventUserPromptSubmit  HarnessEventKind = "user_prompt_submit"
	EventPreToolUse        HarnessEventKind = "pre_tool_use"
	EventPostToolUse       HarnessEventKind = "post_tool_use"
	EventPermissionRequest HarnessEventKind = "permission_request"
	EventWaitingForInput   HarnessEventKind = "waiting_for_input"
	EventStop              HarnessEventKind = "stop"
	EventSessionEnd        HarnessEventKind = "session_end"
)

type HarnessEvent struct {
	Kind HarnessEventKind
}

type EffectKind string

const (
	EffectNotify     EffectKind = "notify"
	EffectMarkUnread EffectKind = "mark_unread"
)

// Effect is work the caller performs after a transition. State is set for
// EffectNotify and names the state being announced.
type Effect struct {
	Kind  EffectKind
	State AgentState
}

type Session struct {
	ID          string
	TaskID      string
	Harness     Harness
	Pane        string
	Model      string
	Effort      string
	State       AgentState
	Unread      bool
	Focused     bool
	WorktreeIDs []string
	Usage       Usage
}

// Apply is the agent state machine. Events that no longer fit the current
// state, such as a PreToolUse delivered after Stop, are ignored.
func (s Session) Apply(ev HarnessEvent) (Session, []Effect) {
	switch ev.Kind {
	case EventSessionStart, EventSessionEnd:
		s.State = StateIdle
		return s, nil
	case EventUserPromptSubmit:
		s.State = StateRunning
		s.Unread = false
		return s, nil
	case EventPreToolUse, EventPostToolUse:
		if s.inTurn() {
			s.State = StateRunning
		}
		return s, nil
	case EventPermissionRequest:
		return s.enterAttention(StatePermission)
	case EventWaitingForInput:
		return s.enterAttention(StateWaiting)
	case EventStop:
		if s.State == StateDone {
			return s, nil
		}
		s.State = StateDone
		effects := []Effect{{Kind: EffectNotify, State: StateDone}}
		if !s.Focused {
			s.Unread = true
			effects = append(effects, Effect{Kind: EffectMarkUnread})
		}
		return s, effects
	}
	return s, nil
}

func (s Session) enterAttention(next AgentState) (Session, []Effect) {
	if !s.inTurn() || s.State == next {
		return s, nil
	}
	s.State = next
	return s, []Effect{{Kind: EffectNotify, State: next}}
}

func (s Session) inTurn() bool {
	return s.State != StateIdle && s.State != StateDone
}

func (s Session) Focus() Session {
	s.Focused = true
	s.Unread = false
	return s
}

func (s Session) Blur() Session {
	s.Focused = false
	return s
}

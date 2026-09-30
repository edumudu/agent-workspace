package domain_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

var allStates = []domain.AgentState{
	domain.StateIdle, domain.StateRunning, domain.StateWaiting, domain.StatePermission, domain.StateDone,
}

var allEvents = []domain.HarnessEventKind{
	domain.EventSessionStart, domain.EventUserPromptSubmit, domain.EventPreToolUse,
	domain.EventPostToolUse, domain.EventPermissionRequest, domain.EventWaitingForInput,
	domain.EventStop, domain.EventSessionEnd,
}

type transition struct {
	next    domain.AgentState
	effects []domain.Effect
}

func notify(s domain.AgentState) domain.Effect {
	return domain.Effect{Kind: domain.EffectNotify, State: s}
}

var markUnread = domain.Effect{Kind: domain.EffectMarkUnread}

var doneEffects = []domain.Effect{notify(domain.StateDone), markUnread}

// expected lists every (state, event) pair for an unfocused session. Tool,
// permission and waiting events seen while idle or done are stale (hooks can
// arrive out of order), so they must not revive the session.
var expected = map[domain.AgentState]map[domain.HarnessEventKind]transition{
	domain.StateIdle: {
		domain.EventSessionStart:      {domain.StateIdle, nil},
		domain.EventUserPromptSubmit:  {domain.StateRunning, nil},
		domain.EventPreToolUse:        {domain.StateIdle, nil},
		domain.EventPostToolUse:       {domain.StateIdle, nil},
		domain.EventPermissionRequest: {domain.StateIdle, nil},
		domain.EventWaitingForInput:   {domain.StateIdle, nil},
		domain.EventStop:              {domain.StateDone, doneEffects},
		domain.EventSessionEnd:        {domain.StateIdle, nil},
	},
	domain.StateRunning: {
		domain.EventSessionStart:      {domain.StateIdle, nil},
		domain.EventUserPromptSubmit:  {domain.StateRunning, nil},
		domain.EventPreToolUse:        {domain.StateRunning, nil},
		domain.EventPostToolUse:       {domain.StateRunning, nil},
		domain.EventPermissionRequest: {domain.StatePermission, []domain.Effect{notify(domain.StatePermission)}},
		domain.EventWaitingForInput:   {domain.StateWaiting, []domain.Effect{notify(domain.StateWaiting)}},
		domain.EventStop:              {domain.StateDone, doneEffects},
		domain.EventSessionEnd:        {domain.StateIdle, nil},
	},
	domain.StateWaiting: {
		domain.EventSessionStart:      {domain.StateIdle, nil},
		domain.EventUserPromptSubmit:  {domain.StateRunning, nil},
		domain.EventPreToolUse:        {domain.StateRunning, nil},
		domain.EventPostToolUse:       {domain.StateRunning, nil},
		domain.EventPermissionRequest: {domain.StatePermission, []domain.Effect{notify(domain.StatePermission)}},
		domain.EventWaitingForInput:   {domain.StateWaiting, nil},
		domain.EventStop:              {domain.StateDone, doneEffects},
		domain.EventSessionEnd:        {domain.StateIdle, nil},
	},
	domain.StatePermission: {
		domain.EventSessionStart:      {domain.StateIdle, nil},
		domain.EventUserPromptSubmit:  {domain.StateRunning, nil},
		domain.EventPreToolUse:        {domain.StateRunning, nil},
		domain.EventPostToolUse:       {domain.StateRunning, nil},
		domain.EventPermissionRequest: {domain.StatePermission, nil},
		domain.EventWaitingForInput:   {domain.StateWaiting, []domain.Effect{notify(domain.StateWaiting)}},
		domain.EventStop:              {domain.StateDone, doneEffects},
		domain.EventSessionEnd:        {domain.StateIdle, nil},
	},
	domain.StateDone: {
		domain.EventSessionStart:      {domain.StateIdle, nil},
		domain.EventUserPromptSubmit:  {domain.StateRunning, nil},
		domain.EventPreToolUse:        {domain.StateDone, nil},
		domain.EventPostToolUse:       {domain.StateDone, nil},
		domain.EventPermissionRequest: {domain.StateDone, nil},
		domain.EventWaitingForInput:   {domain.StateDone, nil},
		domain.EventStop:              {domain.StateDone, nil},
		domain.EventSessionEnd:        {domain.StateIdle, nil},
	},
}

func TestApplyCoversEveryStateEventPair(t *testing.T) {
	for _, state := range allStates {
		for _, ev := range allEvents {
			t.Run(fmt.Sprintf("%s/%s", state, ev), func(t *testing.T) {
				want, ok := expected[state][ev]
				if !ok {
					t.Fatalf("missing expectation for (%s, %s)", state, ev)
				}
				got, effects := domain.Session{State: state}.Apply(domain.HarnessEvent{Kind: ev})
				if got.State != want.next {
					t.Errorf("state = %s, want %s", got.State, want.next)
				}
				if !slices.Equal(effects, want.effects) {
					t.Errorf("effects = %v, want %v", effects, want.effects)
				}
				if got.Unread != slices.Contains(want.effects, markUnread) {
					t.Errorf("unread = %v", got.Unread)
				}
			})
		}
	}
}

func TestApplyUnknownEventIsIgnored(t *testing.T) {
	s := domain.Session{State: domain.StateRunning}
	got, effects := s.Apply(domain.HarnessEvent{Kind: "bogus"})
	if got.State != s.State || effects != nil {
		t.Errorf("got %+v %v, want unchanged", got, effects)
	}
}

func TestOutOfOrderStopBeforePreToolUseStaysDone(t *testing.T) {
	s := domain.Session{State: domain.StateIdle}
	for _, ev := range []domain.HarnessEventKind{
		domain.EventUserPromptSubmit, domain.EventStop, domain.EventPreToolUse, domain.EventPostToolUse,
	} {
		s, _ = s.Apply(domain.HarnessEvent{Kind: ev})
	}
	if s.State != domain.StateDone || !s.Unread {
		t.Errorf("got %s unread=%v, want done unread", s.State, s.Unread)
	}
}

func TestDoneWhileFocusedDoesNotMarkUnread(t *testing.T) {
	s := domain.Session{State: domain.StateRunning}.Focus()
	got, effects := s.Apply(domain.HarnessEvent{Kind: domain.EventStop})
	if got.Unread {
		t.Error("focused session marked unread")
	}
	if !slices.Equal(effects, []domain.Effect{notify(domain.StateDone)}) {
		t.Errorf("effects = %v", effects)
	}
}

func TestFocusClearsUnreadAndBlurKeepsItCleared(t *testing.T) {
	s, _ := domain.Session{State: domain.StateRunning}.Apply(domain.HarnessEvent{Kind: domain.EventStop})
	if !s.Unread {
		t.Fatal("precondition: unread")
	}
	s = s.Focus()
	if s.Unread || !s.Focused {
		t.Errorf("after focus: unread=%v focused=%v", s.Unread, s.Focused)
	}
	s = s.Blur()
	if s.Unread || s.Focused {
		t.Errorf("after blur: unread=%v focused=%v", s.Unread, s.Focused)
	}
	s, _ = s.Apply(domain.HarnessEvent{Kind: domain.EventUserPromptSubmit})
	s, _ = s.Apply(domain.HarnessEvent{Kind: domain.EventStop})
	if !s.Unread {
		t.Error("blurred session not marked unread")
	}
}

func TestUserPromptClearsUnread(t *testing.T) {
	s := domain.Session{State: domain.StateDone, Unread: true}
	got, _ := s.Apply(domain.HarnessEvent{Kind: domain.EventUserPromptSubmit})
	if got.Unread {
		t.Error("prompt left session unread")
	}
}

func TestApplyKeepsOtherFields(t *testing.T) {
	s := domain.Session{ID: "s1", Harness: domain.HarnessCodex, Model: "m", WorktreeIDs: []string{"w"}}
	got, _ := s.Apply(domain.HarnessEvent{Kind: domain.EventUserPromptSubmit})
	if got.ID != "s1" || got.Harness != domain.HarnessCodex || got.Model != "m" || len(got.WorktreeIDs) != 1 {
		t.Errorf("fields lost: %+v", got)
	}
}

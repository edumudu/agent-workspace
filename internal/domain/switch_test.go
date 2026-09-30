package domain

import (
	"reflect"
	"testing"
	"time"
)

var switchT0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestModelSwitchIsSentAtOnceWhenTheAgentIsBetweenTools(t *testing.T) {
	for _, state := range []AgentState{StateIdle, StateDone, StateWaiting} {
		s := Session{State: state}.RequestSwitch(SwitchModel, "opus")
		s, sent := s.Dispatch(switchT0)
		want := []Switch{{Kind: SwitchModel, Value: "opus", SentAt: switchT0}}
		if !reflect.DeepEqual(sent, want) {
			t.Fatalf("%s: sent %+v", state, sent)
		}
		if !reflect.DeepEqual(s.Switches, want) {
			t.Fatalf("%s: session keeps %+v", state, s.Switches)
		}
	}
}

func TestModelSwitchWaitsWhileTheAgentRunsATool(t *testing.T) {
	for _, state := range []AgentState{StateRunning, StatePermission} {
		s := Session{State: state}.RequestSwitch(SwitchModel, "opus")
		s, sent := s.Dispatch(switchT0)
		if len(sent) != 0 || len(s.Switches) != 1 || !s.Switches[0].SentAt.IsZero() {
			t.Fatalf("%s: sent %+v, session %+v", state, sent, s.Switches)
		}
	}
}

func TestModelSwitchQueuedWhileRunningIsSentAtTheNextDone(t *testing.T) {
	s := Session{State: StateRunning}.RequestSwitch(SwitchEffort, "high")
	s, _ = s.Dispatch(switchT0)
	s, _ = s.Apply(HarnessEvent{Kind: EventStop})
	_, sent := s.Dispatch(switchT0.Add(time.Minute))
	if len(sent) != 1 || sent[0].Kind != SwitchEffort || sent[0].SentAt != switchT0.Add(time.Minute) {
		t.Fatalf("sent %+v", sent)
	}
}

func TestModelSwitchQueuedWhileRunningIsSentWhenTheAgentWaits(t *testing.T) {
	s := Session{State: StateRunning}.RequestSwitch(SwitchModel, "opus")
	s, _ = s.Apply(HarnessEvent{Kind: EventWaitingForInput})
	if _, sent := s.Dispatch(switchT0); len(sent) != 1 {
		t.Fatalf("sent %+v", sent)
	}
}

func TestModelSwitchIsNeverSentTwice(t *testing.T) {
	s := Session{State: StateIdle}.RequestSwitch(SwitchModel, "opus")
	s, _ = s.Dispatch(switchT0)
	if _, sent := s.Dispatch(switchT0.Add(time.Second)); len(sent) != 0 {
		t.Fatalf("resent %+v", sent)
	}
}

func TestModelSwitchNewerRequestReplacesAnUnsentOneOfTheSameKind(t *testing.T) {
	s := Session{State: StateRunning}.
		RequestSwitch(SwitchModel, "opus").
		RequestSwitch(SwitchEffort, "low").
		RequestSwitch(SwitchModel, "sonnet")
	want := []Switch{{Kind: SwitchEffort, Value: "low"}, {Kind: SwitchModel, Value: "sonnet"}}
	if !reflect.DeepEqual(s.Switches, want) {
		t.Fatalf("switches %+v", s.Switches)
	}
}

func TestModelSwitchNewRequestRetiresASentUnconfirmedOneOfTheSameKind(t *testing.T) {
	s := sentSession(SwitchModel, "opus").RequestSwitch(SwitchModel, "haiku")
	want := []Switch{{Kind: SwitchModel, Value: "haiku"}}
	if !reflect.DeepEqual(s.Switches, want) {
		t.Fatalf("switches %+v", s.Switches)
	}
}

func TestModelSwitchModelNamesMatchExactlyOrByTheirFirstWord(t *testing.T) {
	cases := []struct {
		requested, reported string
		confirmed           bool
	}{
		{"opus", "Opus 4.7", true},
		{"gpt-5", "GPT-5", true},
		{"gpt-5", "gpt-5-codex", false},
		{"gpt-5-codex", "gpt-5", false},
		{"sonnet", "Opus 4.7", false},
	}
	for _, c := range cases {
		s := sentSession(SwitchModel, c.requested).Report(StatusReport{Model: c.reported})
		if got := len(s.Switches) == 0; got != c.confirmed {
			t.Errorf("%q vs %q: confirmed %v, want %v", c.requested, c.reported, got, c.confirmed)
		}
	}
}

func TestModelSwitchDoesNotMutateTheCallersSession(t *testing.T) {
	before := Session{State: StateIdle}.RequestSwitch(SwitchModel, "opus")
	_, _ = before.Dispatch(switchT0)
	if !before.Switches[0].SentAt.IsZero() {
		t.Fatal("Dispatch changed the original session")
	}
}

func sentSession(kind SwitchKind, value string) Session {
	s, _ := Session{State: StateIdle, Model: "Sonnet 4.6", Effort: "medium"}.RequestSwitch(kind, value).Dispatch(switchT0)
	return s
}

func TestModelSwitchIsConfirmedByTheNextReportShowingIt(t *testing.T) {
	cases := []struct {
		kind   SwitchKind
		value  string
		report StatusReport
	}{
		{SwitchModel, "opus", StatusReport{Model: "Opus 4.7"}},
		{SwitchEffort, "high", StatusReport{Effort: "high"}},
		{SwitchModel, "gpt-5", StatusReport{Model: "GPT-5"}},
	}
	for _, c := range cases {
		s := sentSession(c.kind, c.value).Report(c.report)
		if len(s.Switches) != 0 || s.SwitchWarning {
			t.Fatalf("%v: switches %+v warning %v", c.report, s.Switches, s.SwitchWarning)
		}
	}
}

func TestModelSwitchWarnsWhenTheNextReportStillShowsTheOldValue(t *testing.T) {
	s := sentSession(SwitchModel, "opus").Report(StatusReport{Model: "Sonnet 4.6", Effort: "medium"})
	if !s.SwitchWarning {
		t.Fatal("no warning")
	}
	s = s.Report(StatusReport{Model: "Opus 4.7"})
	if s.SwitchWarning || len(s.Switches) != 0 {
		t.Fatalf("late confirmation left %+v warning %v", s.Switches, s.SwitchWarning)
	}
}

func TestModelSwitchIgnoresReportsThatSayNothingAboutItsKind(t *testing.T) {
	s := sentSession(SwitchModel, "opus").Report(StatusReport{Effort: "high", ContextLeft: 50, HasContext: true})
	if s.SwitchWarning || len(s.Switches) != 1 {
		t.Fatalf("switches %+v warning %v", s.Switches, s.SwitchWarning)
	}
}

func TestModelSwitchNotYetSentIsNotJudgedByReports(t *testing.T) {
	s := Session{State: StateRunning, Model: "Sonnet 4.6"}.RequestSwitch(SwitchModel, "opus")
	s = s.Report(StatusReport{Model: "Sonnet 4.6"})
	if s.SwitchWarning || len(s.Switches) != 1 {
		t.Fatalf("switches %+v warning %v", s.Switches, s.SwitchWarning)
	}
}

func TestModelSwitchNewRequestClearsAnOldWarning(t *testing.T) {
	s := sentSession(SwitchModel, "opus").Report(StatusReport{Model: "Sonnet 4.6"})
	s = s.RequestSwitch(SwitchModel, "haiku")
	if s.SwitchWarning {
		t.Fatal("warning kept")
	}
}

func TestModelSwitchFailedToSendDropsItAndWarns(t *testing.T) {
	s := sentSession(SwitchModel, "opus")
	s = s.SwitchFailed(s.Switches)
	if len(s.Switches) != 0 || !s.SwitchWarning {
		t.Fatalf("switches %+v warning %v", s.Switches, s.SwitchWarning)
	}
}

func TestModelSwitchCommandIsTheHarnessesOwnSlashCommand(t *testing.T) {
	cases := []struct {
		h    Harness
		sw   Switch
		want string
	}{
		{HarnessClaude, Switch{Kind: SwitchModel, Value: "opus"}, "/model opus"},
		{HarnessClaude, Switch{Kind: SwitchEffort, Value: "high"}, "/effort high"},
		{HarnessCodex, Switch{Kind: SwitchModel, Value: "gpt-5"}, "/model gpt-5"},
		{HarnessCodex, Switch{Kind: SwitchEffort, Value: "low"}, "/effort low"},
	}
	for _, c := range cases {
		if got := SwitchCommand(c.h, c.sw); got != c.want {
			t.Errorf("%s %+v: %q, want %q", c.h, c.sw, got, c.want)
		}
	}
}

func TestModelSwitchChoicesExistForBothHarnesses(t *testing.T) {
	for _, h := range []Harness{HarnessClaude, HarnessCodex} {
		for _, kind := range []SwitchKind{SwitchModel, SwitchEffort} {
			if len(SwitchChoices(h, kind)) == 0 {
				t.Errorf("%s %s: no choices", h, kind)
			}
		}
	}
}

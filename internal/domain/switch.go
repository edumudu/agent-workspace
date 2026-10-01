package domain

import (
	"slices"
	"strings"
	"time"
)

type SwitchKind string

const (
	SwitchModel  SwitchKind = "model"
	SwitchEffort SwitchKind = "effort"
)

// Switch is a model or effort change the user asked for. SentAt is zero
// while it waits for the agent to be between tools.
type Switch struct {
	Kind   SwitchKind
	Value  string
	SentAt time.Time
}

func (sw Switch) sent() bool { return !sw.SentAt.IsZero() }

func SwitchSupported(h Harness) bool { return h == HarnessClaude || h == HarnessCodex }

func SwitchChoices(h Harness, kind SwitchKind) []string {
	switch {
	case !SwitchSupported(h):
		return nil
	case kind == SwitchEffort:
		return []string{"low", "medium", "high", "xhigh", "max"}
	case h == HarnessCodex:
		return []string{"gpt-6.1-sol", "gpt-6-astra", "gpt-6-sol", "gpt-6-luna", "gpt-5.5"}
	}
	return []string{"opus", "sonnet", "haiku"}
}

// SwitchCommand is what gets typed first. Codex's `/model` takes no value and
// opens a picker that CodexPickerKeys then drives; it has no `/effort`.
func SwitchCommand(h Harness, sw Switch) string {
	if h == HarnessCodex {
		return "/model"
	}
	return "/" + string(sw.Kind) + " " + sw.Value
}

// RequestSwitch queues a switch, replacing any earlier one of the same kind,
// and clears the warning of an earlier failed switch.
func (s Session) RequestSwitch(kind SwitchKind, value string) Session {
	kept := make([]Switch, 0, len(s.Switches)+1)
	for _, sw := range s.Switches {
		if sw.Kind != kind {
			kept = append(kept, sw)
		}
	}
	s.Switches = append(kept, Switch{Kind: kind, Value: value})
	s.SwitchWarning = false
	return s
}

// Dispatch marks the queued switches as sent at now and returns them, but
// only while the agent is not in a turn or is waiting on the user. Typing into
// a running agent, or into a permission prompt, would land in the wrong place.
func (s Session) Dispatch(now time.Time) (Session, []Switch) {
	if !s.AcceptsSwitch() {
		return s, nil
	}
	var out []Switch
	next := slices.Clone(s.Switches)
	for i, sw := range next {
		if !sw.sent() {
			next[i].SentAt = now
			out = append(out, next[i])
		}
	}
	s.Switches = next
	return s, out
}

// AcceptsSwitch is true when typing into the pane cannot land in a tool run
// or answer a permission prompt.
func (s Session) AcceptsSwitch() bool {
	switch s.State {
	case StateIdle, StateDone, StateWaiting:
		return true
	}
	return false
}

// Requeue takes back switches that were dispatched but not typed, because the
// session stopped accepting them in between.
func (s Session) Requeue(unsent []Switch) Session {
	next := slices.Clone(s.Switches)
	for i, sw := range next {
		if slices.Contains(unsent, sw) {
			next[i].SentAt = time.Time{}
		}
	}
	s.Switches = next
	return s
}

func (s Session) SwitchFailed(failed []Switch) Session {
	s.Switches = slices.DeleteFunc(slices.Clone(s.Switches), func(sw Switch) bool { return slices.Contains(failed, sw) })
	s.SwitchWarning = true
	return s
}

// confirmSwitches judges every sent switch against the report. A report that
// says nothing about a switch's kind leaves it pending; one that shows the
// old value raises the warning but keeps the switch, so a later report that
// shows the new value still clears it. A report stamped before the switch was
// sent cannot show it yet: Codex's rollout only has the new values once a
// turn starts after the switch.
func (s Session) confirmSwitches(r StatusReport) Session {
	if len(s.Switches) == 0 {
		return s
	}
	kept := make([]Switch, 0, len(s.Switches))
	warn := false
	for _, sw := range s.Switches {
		reported := r.Model
		if sw.Kind == SwitchEffort {
			reported = r.Effort
		}
		switch {
		case !sw.sent() || reported == "":
			kept = append(kept, sw)
		case switchShows(sw, reported):
		case !r.At.IsZero() && r.At.Before(sw.SentAt):
			kept = append(kept, sw)
		default:
			kept = append(kept, sw)
			warn = true
		}
	}
	s.Switches = kept
	if warn {
		s.SwitchWarning = true
	} else if !slices.ContainsFunc(kept, Switch.sent) {
		s.SwitchWarning = false
	}
	return s
}

// switchShows matches the whole reported name, its first word, or the name
// without a trailing version: Claude reports "Opus 4.7" or "opus-5.5" for the
// alias "opus", while "gpt-5" must not match "gpt-5-codex".
func switchShows(sw Switch, reported string) bool {
	want, got := strings.ToLower(sw.Value), strings.ToLower(reported)
	if sw.Kind == SwitchEffort {
		return want == got
	}
	first, _, _ := strings.Cut(got, " ")
	return got == want || first == want || withoutVersion(got) == want
}

// withoutVersion drops a trailing "-<digits and dots>": "opus-5.5" is "opus".
func withoutVersion(name string) string {
	i := strings.LastIndex(name, "-")
	if i <= 0 || strings.Trim(name[i+1:], "0123456789.") != "" || name[i+1:] == "" {
		return name
	}
	return name[:i]
}

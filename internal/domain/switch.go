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

func SwitchChoices(h Harness, kind SwitchKind) []string {
	switch {
	case h == HarnessCodex && kind == SwitchModel:
		return []string{"gpt-5-codex", "gpt-5"}
	case h == HarnessCodex:
		return []string{"minimal", "low", "medium", "high"}
	case kind == SwitchModel:
		return []string{"opus", "sonnet", "haiku"}
	}
	return []string{"low", "medium", "high", "xhigh", "max"}
}

// SwitchCommand is the slash command that makes the harness apply sw.
func SwitchCommand(_ Harness, sw Switch) string {
	return "/" + string(sw.Kind) + " " + sw.Value
}

// RequestSwitch queues a switch, replacing an unsent one of the same kind,
// and clears the warning of an earlier failed switch.
func (s Session) RequestSwitch(kind SwitchKind, value string) Session {
	kept := make([]Switch, 0, len(s.Switches)+1)
	for _, sw := range s.Switches {
		if sw.Kind != kind || sw.sent() {
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
	switch s.State {
	case StateIdle, StateDone, StateWaiting:
	default:
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

// SwitchFailed drops switches that could not be typed into the pane and
// raises the warning.
func (s Session) SwitchFailed(failed []Switch) Session {
	s.Switches = slices.DeleteFunc(slices.Clone(s.Switches), func(sw Switch) bool { return slices.Contains(failed, sw) })
	s.SwitchWarning = true
	return s
}

// confirmSwitches judges every sent switch against the report. A report that
// says nothing about a switch's kind leaves it pending; one that shows the
// old value raises the warning but keeps the switch, so a later report that
// shows the new value still clears it.
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

// switchShows accepts a reported model that contains the requested one, since
// Claude reports "Opus 4.7" for the alias "opus".
func switchShows(sw Switch, reported string) bool {
	want, got := strings.ToLower(sw.Value), strings.ToLower(reported)
	if sw.Kind == SwitchEffort {
		return want == got
	}
	return strings.Contains(got, want)
}

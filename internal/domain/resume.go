package domain

import (
	"encoding/json"
	"sort"
	"time"
)

func ResumeIDFromHook(payload []byte) string {
	var p struct {
		SessionID string `json:"session_id"`
		ThreadID  string `json:"thread-id"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	if p.SessionID != "" {
		return p.SessionID
	}
	return p.ThreadID
}

func (s Session) Resumable() bool {
	return s.Ended && s.ResumeID != "" && s.Dir != ""
}

func (s Session) Resumed(pane string) Session {
	s.Ended = false
	s.Pane = pane
	s.State = StateIdle
	s.Unread = false
	return s
}

func ResumableSessions(sessions []Session, events map[string][]SessionEvent) []Session {
	last := func(id string) time.Time {
		var t time.Time
		for _, ev := range events[id] {
			if ev.At.After(t) {
				t = ev.At
			}
		}
		return t
	}
	var out []Session
	for _, s := range sessions {
		if s.Resumable() {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return last(out[i].ID).After(last(out[j].ID)) })
	return out
}

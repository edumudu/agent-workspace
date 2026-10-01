package domain

import (
	"strings"
	"time"
)

const CoalesceWindow = 10 * time.Second

type Banner struct {
	Title string
	Body  string
	State AgentState
	Sound string
}

var bannerBody = map[AgentState]string{
	StatePermission: "needs permission",
	StateWaiting:    "waiting",
	StateDone:       "done",
}

func BannerFor(s Session, name string, e Effect) (Banner, bool) {
	body, ok := bannerBody[e.State]
	if e.Kind != EffectNotify || s.Muted || !ok {
		return Banner{}, false
	}
	title := strings.TrimSpace(name)
	if title == "" {
		title = string(s.Harness)
	}
	return Banner{Title: title, Body: body, State: e.State}, true
}

func (s Session) SetMuted(muted bool) Session {
	s.Muted = muted
	return s
}

type Coalescer struct {
	last map[string]time.Time
}

func NewCoalescer() *Coalescer {
	return &Coalescer{last: map[string]time.Time{}}
}

func (c *Coalescer) Allow(sessionID string, now time.Time) bool {
	if at, ok := c.last[sessionID]; ok && now.Sub(at) < CoalesceWindow {
		return false
	}
	c.last[sessionID] = now
	return true
}

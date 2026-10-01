package domain

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const CoalesceWindow = 10 * time.Second

type Banner struct {
	Title string
	Body  string
	State AgentState
	Sound string
	// why: one banner per group is kept, so a session's newer banner replaces its older one.
	Group string
	// why: the bundle id of the terminal running the client, brought to the front on click.
	Terminal string
}

const (
	MaxBannerTitle  = 80
	MaxBannerBody   = 120
	maxBannerBranch = 29
)

var bannerBody = map[AgentState]string{
	StatePermission: "needs permission",
	StateWaiting:    "waiting",
	StateDone:       "done",
}

// why: Events may hold every session's recent events; only Session's own count.
type BannerInput struct {
	Session   Session
	Name      string
	Effect    Effect
	Worktrees []Worktree
	Events    []SessionEvent
	Now       time.Time
}

func BannerFor(in BannerInput) (Banner, bool) {
	fallback, ok := bannerBody[in.Effect.State]
	if in.Effect.Kind != EffectNotify || in.Session.Muted || !ok {
		return Banner{}, false
	}
	var own []SessionEvent
	for _, ev := range in.Events {
		if ev.SessionID == in.Session.ID {
			own = append(own, ev)
		}
	}
	body := bannerDetail(in.Effect.State, own, in.Now)
	if body == "" {
		body = fallback
	}
	return Banner{
		Title: cutRunes(bannerTitle(in.Session, in.Name, in.Worktrees), MaxBannerTitle),
		Body:  body,
		State: in.Effect.State,
	}, true
}

func bannerTitle(s Session, name string, worktrees []Worktree) string {
	title := strings.TrimSpace(name)
	if title == "" {
		title = string(s.Harness)
	}
	if len(worktrees) == 0 {
		return title
	}
	where := path.Base(worktrees[0].Repo)
	if b := worktrees[0].Branch; b != "" {
		where += "@" + cutRunes(b, maxBannerBranch)
	}
	if more := len(worktrees) - 1; more > 0 {
		where += fmt.Sprintf(" +%d", more)
	}
	suffix := " · " + where
	room := MaxBannerTitle - utf8.RuneCountInString(suffix)
	if room <= 0 {
		return suffix
	}
	return cutRunes(title, room) + suffix
}

func bannerDetail(state AgentState, events []SessionEvent, now time.Time) string {
	turn := sinceLastPrompt(events)
	switch state {
	case StatePermission:
		ev, ok := latest(turn, EventPermissionRequest)
		if !ok {
			return ""
		}
		need := ev.Text
		if ev.Tool != "" {
			need = ev.Tool
			if ev.Detail != "" {
				need += ": " + ev.Detail
			}
		}
		return bannerLine("needs permission: ", need, "")
	case StateWaiting:
		if q := lastQuestion(turn); q != "" {
			return bannerLine("asks: ", q, "")
		}
		if ev, ok := latest(turn, EventWaitingForInput); ok {
			return bannerLine("waiting: ", ev.Text, "")
		}
	case StateDone:
		return doneDetail(events, turn, now)
	}
	return ""
}

func doneDetail(events, turn []SessionEvent, now time.Time) string {
	took := elapsed(events, now)
	var said string
	if ev, ok := latest(turn, EventStop); ok {
		said = firstLine(strings.TrimSpace(ev.Text))
	}
	switch {
	case said == "" && took == "":
		return ""
	case said == "":
		return "done in " + took
	case isLimitMessage(said):
		return bannerLine("usage limit: ", said, "")
	case strings.HasPrefix(said, "API Error"):
		return bannerLine("error: ", said, "")
	case took == "":
		return bannerLine("", said, "")
	}
	return bannerLine("", said, " ("+took+")")
}

func isLimitMessage(s string) bool {
	s = strings.ToLower(s)
	for _, k := range []string{"usage limit", "rate limit", "hit your limit"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func elapsed(events []SessionEvent, now time.Time) string {
	if now.IsZero() {
		return ""
	}
	prompt, ok := latest(events, EventUserPromptSubmit)
	if !ok {
		return ""
	}
	d := now.Sub(prompt.At).Truncate(time.Second)
	switch {
	case d < time.Second:
		return ""
	case d < time.Hour:
		return d.String()
	}
	return strings.TrimSuffix(d.Truncate(time.Minute).String(), "0s")
}

// why: the suffix (elapsed time) survives the cut, so a long message never hides how long the turn took.
func bannerLine(prefix, text, suffix string) string {
	text = maskSecrets(firstLine(strings.TrimSpace(text)))
	if text == "" {
		return ""
	}
	room := MaxBannerBody - utf8.RuneCountInString(prefix) - utf8.RuneCountInString(suffix)
	return prefix + cutRunes(text, room) + suffix
}

// why: compiled on first use, since package init runs on every hook.
var secretPatterns = sync.OnceValue(func() [4]*regexp.Regexp {
	return [4]*regexp.Regexp{
		regexp.MustCompile(`(?i)\b([a-z0-9_]*(?:token|secret|password|passwd|api[_-]?key)[a-z0-9_]*)(\s*[=:]\s*)(?:"[^"]*"|'[^']*'|\S+)`),
		regexp.MustCompile(`(?i)\b(authorization|cookie)(\s*:\s*)(?:(?:bearer|basic|token)\s+)?[^\s"']+`),
		regexp.MustCompile(`(://[^/\s:@]+:)[^@\s/]+@`),
		regexp.MustCompile(`\b(?:sk-|ghp_|gho_|ghs_|ghu_|github_pat_|xox[abpr]-|AKIA)[A-Za-z0-9_\-]{6,}`),
	}
})

func maskSecrets(s string) string {
	p := secretPatterns()
	s = p[0].ReplaceAllString(s, "${1}${2}…")
	s = p[1].ReplaceAllString(s, "${1}${2}…")
	s = p[2].ReplaceAllString(s, "${1}…@")
	return p[3].ReplaceAllString(s, "…")
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

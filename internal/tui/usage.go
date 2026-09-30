package tui

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// limitLines is one row per harness that has reported limits, under the top
// bar. With no data it is empty: the slot is hidden, never shown as zero.
func (m Model) limitLines() []string {
	sessions := make([]domain.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	quotas := domain.Quotas(sessions)
	now := m.opts.Now()
	var lines []string
	for _, h := range []domain.Harness{domain.HarnessClaude, domain.HarnessCodex} {
		if line, ok := m.limitLine(h, quotas, now); ok {
			lines = append(lines, line)
		}
	}
	return lines
}

func (m Model) limitLine(h domain.Harness, quotas []domain.Quota, now time.Time) (string, bool) {
	s := m.styles
	tag := piece{s.blue.Bold(true), " CC "}
	if h == domain.HarnessCodex {
		tag = piece{s.teal.Bold(true), " CX "}
	}
	red := lipgloss.NewStyle().Foreground(lipgloss.Color(m.opts.Theme.Red)).Bold(true)
	left := []piece{tag}
	var oldest time.Duration
	for _, q := range quotas {
		if q.Harness != h {
			continue
		}
		text, figure := s.text, s.text
		if q.Stale(now) {
			text, figure = s.dim, s.dim
			oldest = max(oldest, q.Age(now))
		}
		if q.Low() {
			figure = red
		}
		if len(left) > 1 {
			left = append(left, piece{s.text, "  "})
		}
		left = append(left, piece{text, domain.WindowLabel(q.Window) + " "}, piece{figure, fmt.Sprintf("%d%%", q.LeftPercent)})
		if reset := untilReset(q.ResetsAt, now); reset != "" {
			left = append(left, piece{text, " " + reset})
		}
	}
	if len(left) == 1 {
		return "", false
	}
	var right []piece
	if oldest > 0 {
		right = []piece{{s.dim, shortDuration(oldest) + " ago "}}
	}
	return m.line(false, left, right), true
}

func untilReset(resetsAt int64, now time.Time) string {
	if resetsAt == 0 {
		return ""
	}
	d := time.Unix(resetsAt, 0).Sub(now)
	if d <= 0 {
		return ""
	}
	if d < time.Minute {
		return "<1m"
	}
	return shortDuration(d)
}

func shortDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d >= time.Hour:
		if m := int(d % time.Hour / time.Minute); m > 0 {
			return fmt.Sprintf("%dh%dm", int(d/time.Hour), m)
		}
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}

package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// LowQuotaLeft is the percent left below which the top bar turns red.
	LowQuotaLeft = 20
	// WarnQuotaLeft is the percent left below which the new-session dialog warns.
	WarnQuotaLeft = 25
	// StaleQuotaAfter is how old a report may get before it is shown dimmed.
	StaleQuotaAfter = 15 * time.Minute
)

// Quota is what one harness last reported for one usage window.
type Quota struct {
	Harness     Harness
	Window      string
	LeftPercent int
	ResetsAt    int64
	ReportedAt  time.Time
}

func (q Quota) Low() bool { return q.LeftPercent < LowQuotaLeft }

func (q Quota) Age(now time.Time) time.Duration { return max(now.Sub(q.ReportedAt), 0) }

func (q Quota) Stale(now time.Time) bool { return q.Age(now) > StaleQuotaAfter }

// Quotas folds the sessions' limits into one figure per harness and window,
// taking each from the newest report. Accounts are shared by every session of
// a harness, so the newest report is the best guess. Harnesses come in
// declaration order, windows shortest first.
func Quotas(sessions []Session) []Quota {
	type key struct {
		harness Harness
		window  string
	}
	best := map[key]Quota{}
	for _, s := range sessions {
		for _, l := range s.Limits {
			k := key{s.Harness, l.Window}
			if old, ok := best[k]; ok && old.ReportedAt.After(s.LimitsAt) {
				continue
			}
			best[k] = Quota{
				Harness:     s.Harness,
				Window:      l.Window,
				LeftPercent: min(max(100-l.UsedPercent, 0), 100),
				ResetsAt:    l.ResetsAt,
				ReportedAt:  s.LimitsAt,
			}
		}
	}
	out := make([]Quota, 0, len(best))
	for _, q := range best {
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Harness != b.Harness {
			return harnessRank(a.Harness) < harnessRank(b.Harness)
		}
		if da, db := windowDuration(a.Window), windowDuration(b.Window); da != db {
			return da < db
		}
		return a.Window < b.Window
	})
	return out
}

func harnessRank(h Harness) int {
	if h == HarnessClaude {
		return 0
	}
	return 1
}

const (
	fiveHours = 5 * time.Hour
	sevenDays = 7 * 24 * time.Hour
)

// windowDuration orders windows. Claude's per-model windows are seven-day
// windows; a window it does not know sorts last.
func windowDuration(window string) time.Duration {
	switch {
	case window == "five_hour":
		return fiveHours
	case strings.HasPrefix(window, "seven_day"):
		return sevenDays
	}
	var minutes int
	if _, err := fmt.Sscanf(window, "%dm", &minutes); err == nil && strings.HasSuffix(window, "m") {
		return time.Duration(minutes) * time.Minute
	}
	return 1<<63 - 1
}

// WindowNameForMinutes names a window a harness reports by length, so Codex's
// windows line up with Claude's.
func WindowNameForMinutes(minutes int) string {
	switch time.Duration(minutes) * time.Minute {
	case fiveHours:
		return "five_hour"
	case sevenDays:
		return "seven_day"
	}
	return fmt.Sprintf("%dm", minutes)
}

// WindowLabel is the short name the top bar shows: 5h, 7d, "7d opus".
func WindowLabel(window string) string {
	switch {
	case window == "five_hour":
		return "5h"
	case window == "seven_day":
		return "7d"
	case strings.HasPrefix(window, "seven_day_"):
		return "7d " + strings.TrimPrefix(window, "seven_day_")
	}
	d := windowDuration(window)
	if d == 1<<63-1 {
		return window
	}
	minutes := int(d / time.Minute)
	switch {
	case minutes%60 == 0:
		return fmt.Sprintf("%dh", minutes/60)
	case minutes > 60:
		return fmt.Sprintf("%dh%dm", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%dm", minutes)
}

// SwitchAdvice is the new-session dialog's low-quota warning. OtherShortest
// is nil when the other harness has reported nothing, which leaves the
// dialog nothing to offer a switch on.
type SwitchAdvice struct {
	Low           Quota
	Other         Harness
	OtherShortest *Quota
}

// Advise returns a warning when the shortest window of the chosen harness has
// less than WarnQuotaLeft percent left. A harness with no figures gets none.
func Advise(quotas []Quota, chosen Harness) (SwitchAdvice, bool) {
	low, ok := ShortestQuota(quotas, chosen)
	if !ok || low.LeftPercent >= WarnQuotaLeft {
		return SwitchAdvice{}, false
	}
	other := HarnessClaude
	if chosen == HarnessClaude {
		other = HarnessCodex
	}
	advice := SwitchAdvice{Low: low, Other: other}
	if q, ok := ShortestQuota(quotas, other); ok {
		advice.OtherShortest = &q
	}
	return advice, true
}

// ShortestQuota is the harness's shortest window; among equally long ones,
// the one with the least left.
func ShortestQuota(quotas []Quota, h Harness) (Quota, bool) {
	var best Quota
	found := false
	for _, q := range quotas {
		if q.Harness != h {
			continue
		}
		if !found || windowDuration(q.Window) < windowDuration(best.Window) ||
			windowDuration(q.Window) == windowDuration(best.Window) && q.LeftPercent < best.LeftPercent {
			best, found = q, true
		}
	}
	return best, found
}

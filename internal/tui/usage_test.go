package tui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/tui"
)

const (
	redEscape     = "38;2;210;15;57"
	overlayEscape = "38;2;156;160;176"
)

func limitedState(reports ...domain.Session) rpc.State {
	st := fixture(len(reports), 0)
	for i := range reports {
		st.Sessions[i].Harness = reports[i].Harness
		st.Sessions[i].Limits = reports[i].Limits
		st.Sessions[i].LimitsAt = reports[i].LimitsAt
	}
	return st
}

func resetIn(d time.Duration) int64 { return clock().Add(d).Unix() }

func limitLines(m tui.Model) []string {
	var out []string
	for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.HasPrefix(line, " CC ") || strings.HasPrefix(line, " CX ") {
			out = append(out, strings.TrimRight(line, " "))
		}
	}
	return out
}

func TestTopBarShowsEachWindowWithPercentLeftAndTimeToReset(t *testing.T) {
	st := limitedState(
		domain.Session{Harness: domain.HarnessClaude, LimitsAt: clock(), Limits: []domain.RateLimit{
			{Window: "five_hour", UsedPercent: 57, ResetsAt: resetIn(2*time.Hour + 10*time.Minute)},
			{Window: "seven_day", UsedPercent: 71, ResetsAt: resetIn(3*24*time.Hour + 4*time.Hour)},
			{Window: "seven_day_opus", UsedPercent: 12, ResetsAt: resetIn(45 * time.Minute)},
		}},
		domain.Session{Harness: domain.HarnessCodex, LimitsAt: clock(), Limits: []domain.RateLimit{
			{Window: "five_hour", UsedPercent: 41, ResetsAt: resetIn(20 * time.Second)},
		}},
	)
	got := limitLines(newModel(&st, nil))
	want := []string{
		" CC 5h 43% 2h10m  7d 29% 3d  7d opus 88% 45m",
		" CX 5h 59% <1m",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestTopBarHidesTheLimitsSlotWithoutData(t *testing.T) {
	st := fixture(2, 1)
	if got := limitLines(newModel(&st, nil)); len(got) != 0 {
		t.Fatalf("limit lines %q", got)
	}
}

func TestLowWindowsTurnRedBelowTwentyPercentLeft(t *testing.T) {
	st := limitedState(domain.Session{Harness: domain.HarnessClaude, LimitsAt: clock(), Limits: []domain.RateLimit{
		{Window: "five_hour", UsedPercent: 81},
		{Window: "seven_day", UsedPercent: 80},
	}})
	var five, seven string
	for _, line := range strings.Split(newModel(&st, nil).View().Content, "\n") {
		if strings.Contains(ansi.Strip(line), "5h 19%") {
			five = line
		}
	}
	if !strings.Contains(five, redEscape) {
		t.Fatalf("19%% left is not red: %q", five)
	}
	st.Sessions[0].Limits[0].UsedPercent = 80
	for _, line := range strings.Split(newModel(&st, nil).View().Content, "\n") {
		if strings.Contains(ansi.Strip(line), "5h 20%") {
			seven = line
		}
	}
	if seven == "" || strings.Contains(seven, redEscape) {
		t.Fatalf("20%% left is red: %q", seven)
	}
}

func TestValuesOlderThanFifteenMinutesAreDimmedWithTheirAge(t *testing.T) {
	stale := clock().Add(-20 * time.Minute)
	st := limitedState(domain.Session{Harness: domain.HarnessClaude, LimitsAt: stale, Limits: []domain.RateLimit{
		{Window: "five_hour", UsedPercent: 40},
	}})
	m := newModel(&st, nil)
	var line string
	for _, l := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(ansi.Strip(l), "5h 60%") {
			line = l
		}
	}
	if !strings.Contains(ansi.Strip(line), "20m ago") || !strings.Contains(line, overlayEscape) {
		t.Fatalf("stale line %q", line)
	}

	st.Sessions[0].LimitsAt = clock().Add(-15 * time.Minute)
	m = newModel(&st, nil)
	for _, l := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(ansi.Strip(l), "5h 60%") && (strings.Contains(ansi.Strip(l), "ago") || strings.Contains(l, overlayEscape)) {
			t.Fatalf("15m old value is dimmed: %q", l)
		}
	}
}

func TestLimitsFollowTheNewestReportAcrossDiffs(t *testing.T) {
	st := limitedState(domain.Session{Harness: domain.HarnessClaude, LimitsAt: clock(), Limits: []domain.RateLimit{{Window: "five_hour", UsedPercent: 10}}})
	m := newModel(&st, nil)
	next := st.Sessions[0]
	next.Limits = []domain.RateLimit{{Window: "five_hour", UsedPercent: 30}}
	next.LimitsAt = clock().Add(time.Minute)
	m = update(m, tui.DiffMsg(rpc.Diff{Session: &next}))
	if got := limitLines(m); len(got) != 1 || !strings.Contains(got[0], "5h 70%") {
		t.Fatalf("got %q", got)
	}
}

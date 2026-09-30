package domain

import (
	"reflect"
	"testing"
)

func TestReportSetsModelEffortAndUsage(t *testing.T) {
	limits := []RateLimit{{Window: "five_hour", UsedPercent: 57}, {Window: "seven_day", UsedPercent: 71}, {Window: "seven_day_opus", UsedPercent: 12}}
	s := Session{Model: "old", Effort: "low", State: StateRunning}
	got := s.Report(StatusReport{Model: "Opus", Effort: "high", ContextLeft: 94, HasContext: true, Limits: limits})
	if got.Model != "Opus" || got.Effort != "high" || got.State != StateRunning {
		t.Fatalf("got %+v", got)
	}
	if want := (Usage{ContextLeftPercent: 94, LimitUsedPercent: 71}); got.Usage != want {
		t.Fatalf("usage = %+v, want %+v", got.Usage, want)
	}
	if !reflect.DeepEqual(got.Limits, limits) {
		t.Fatalf("limits = %+v", got.Limits)
	}
}

func TestReportKeepsWhatTheStatusLineDoesNotKnowYet(t *testing.T) {
	s := Session{Model: "Opus", Effort: "high", Usage: Usage{ContextLeftPercent: 40, LimitUsedPercent: 30}, Limits: []RateLimit{{Window: "five_hour", UsedPercent: 30}}}
	got := s.Report(StatusReport{})
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("got %+v, want %+v", got, s)
	}
}

func TestReportWithZeroContextLeftIsKnown(t *testing.T) {
	got := Session{Usage: Usage{ContextLeftPercent: 40}}.Report(StatusReport{HasContext: true})
	if got.Usage.ContextLeftPercent != 0 {
		t.Fatalf("context left = %d", got.Usage.ContextLeftPercent)
	}
}

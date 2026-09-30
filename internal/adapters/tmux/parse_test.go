package tmux

import (
	"slices"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
)

func TestParsePanesKeepsOnlyManagedPanes(t *testing.T) {
	out := "%0\t0\t\n" +
		"%1\t0\t1\n" +
		"%2\t1\t1\n" +
		"\n" +
		"%3\t0\t\n"
	want := []app.PaneInfo{
		{ID: "%1", Alive: true},
		{ID: "%2", Alive: false},
	}
	if got := parsePanes(out); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTrimCaptureDropsTrailingBlankLines(t *testing.T) {
	tests := map[string]string{
		"a\nb\n\n\n":  "a\nb",
		"a\n\nb\n":    "a\n\nb",
		"\n\n":        "",
		"  a  \n   \n": "  a  ",
	}
	for in, want := range tests {
		if got := trimCapture(in); got != want {
			t.Errorf("trimCapture(%q) = %q, want %q", in, got, want)
		}
	}
}

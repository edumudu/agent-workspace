package main

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		fixture string
		want    []string
	}{
		{"good", nil},
		{"long", []string{"AGENTS.md: 4 lines, over the 3-line limit"}},
		{"nosymlink", []string{"sub/CLAUDE.md: missing, want a symlink to AGENTS.md"}},
		{"copied", []string{"sub/CLAUDE.md: not a symlink to AGENTS.md"}},
		{"badlink", []string{
			"AGENTS.md:3: broken link missing.md",
			"ARCHITECTURE.md:3: broken link docs/x.md",
			"sub/AGENTS.md:3: broken link ../nope/AGENTS.md#a",
		}},
		{"skipped", nil},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			got, err := check(filepath.Join("testdata", tt.fixture), 3)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("findings = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunExitCode(t *testing.T) {
	tests := []struct {
		fixture string
		want    int
	}{
		{"good", 0},
		{"badlink", 1},
		{"missing-dir", 2},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			var out bytes.Buffer
			got := run(filepath.Join("testdata", tt.fixture), 3, &out)
			if got != tt.want {
				t.Errorf("run = %d, want %d (%s)", got, tt.want, out.String())
			}
			if tt.want == 1 && !strings.Contains(out.String(), "broken link") {
				t.Errorf("output %q names no finding", out.String())
			}
		})
	}
}

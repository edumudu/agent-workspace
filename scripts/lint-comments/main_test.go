package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCheckFile(t *testing.T) {
	tests := []struct {
		fixture   string
		wantLines []int
	}{
		{"bad_body.go.txt", []int{5}},
		{"why.go.txt", nil},
		{"directive.go.txt", []int{6}},
		{"todo.go.txt", []int{3}},
		{"nolint_no_reason.go.txt", []int{4}},
		{"unmarked.go.txt", []int{1, 6, 9, 12, 15, 18, 21, 24}},
		{"marked.go.txt", nil},
		{"trailing.go.txt", []int{3, 5, 8, 11}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			findings, err := checkFile(tt.fixture, src)
			if err != nil {
				t.Fatal(err)
			}
			var lines []int
			for _, f := range findings {
				lines = append(lines, f.line)
			}
			if !slices.Equal(lines, tt.wantLines) {
				t.Errorf("finding lines = %v, want %v (%v)", lines, tt.wantLines, findings)
			}
		})
	}
}

func TestRunExitCode(t *testing.T) {
	tests := []struct {
		fixture string
		want    int
	}{
		{"bad_body.go.txt", 1},
		{"why.go.txt", 0},
		{"unmarked.go.txt", 1},
		{"marked.go.txt", 0},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			dir := t.TempDir()
			src, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "f.go"), src, 0o644); err != nil {
				t.Fatal(err)
			}
			if got := run([]string{dir}, os.Stderr); got != tt.want {
				t.Errorf("run exit = %d, want %d", got, tt.want)
			}
		})
	}
}

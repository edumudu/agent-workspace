package domain

import "testing"

func TestNewerReleaseComparesSemverWithPrereleases(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.0-alpha.1", "v0.1.0-alpha.2", true},
		{"v0.1.0-alpha.2", "v0.1.0-alpha.2", false},
		{"v0.1.0-alpha.10", "v0.1.0-alpha.9", false},
		{"v0.1.0-alpha.9", "v0.1.0-alpha.10", true},
		{"v0.1.0-alpha.3", "v0.1.0", true},
		{"v0.1.0", "v0.1.0-alpha.3", false},
		{"v0.1.0-alpha.1", "v0.2.0-alpha.1", true},
		{"v0.2.0-alpha.1", "v0.1.9", false},
		{"v0.1.1", "v1.0.0", true},
		{"v0.1.0", "v0.1.1", true},
		{"v0.1.1", "v0.1.0", false},
		{"0.1.0", "v0.1.1", true},
		{"v0.1.0-alpha.1", "v0.1.0-beta.1", true},
		{"v0.1.0-beta.1", "v0.1.0-alpha.1", false},
		{"v0.1.0-alpha", "v0.1.0-alpha.1", true},
		{"v0.1.0-alpha.1", "v0.1.0-alpha", false},
		{"v0.1.0-alpha.1", "v0.1.0-1", false},
		{"v0.1.0-1", "v0.1.0-alpha.1", true},
		{"dev", "v0.1.0", false},
		{"v0.1.0", "garbage", false},
		{"v0.1", "v0.1.1", false},
		{"", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := NewerRelease(c.current, c.latest); got != c.want {
			t.Errorf("NewerRelease(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

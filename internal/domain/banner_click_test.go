package domain

import "testing"

func TestNotifyTerminalBundle(t *testing.T) {
	cases := []struct {
		bundle, program, want string
	}{
		{"com.mitchellh.ghostty", "ghostty", "com.mitchellh.ghostty"},
		{"com.example.Custom", "", "com.example.Custom"},
		{"", "Apple_Terminal", "com.apple.Terminal"},
		{"", "iTerm.app", "com.googlecode.iterm2"},
		{"", "ghostty", "com.mitchellh.ghostty"},
		{"", "WezTerm", "com.github.wez.wezterm"},
		{"", "vscode", "com.microsoft.VSCode"},
		{"", "WarpTerminal", "dev.warp.Warp-Stable"},
		{"", "tmux", ""},
		{"", "", ""},
		{"  ", "", ""},
	}
	for _, c := range cases {
		if got := TerminalBundle(c.bundle, c.program); got != c.want {
			t.Errorf("TerminalBundle(%q, %q) = %q, want %q", c.bundle, c.program, got, c.want)
		}
	}
}

func TestNotifyBannerStale(t *testing.T) {
	cases := []struct {
		prev, next AgentState
		want       bool
	}{
		{StatePermission, StateRunning, true},
		{StateWaiting, StateRunning, true},
		{StateDone, StateRunning, true},
		{StateDone, StateIdle, true},
		{StateRunning, StateRunning, false},
		{StateIdle, StateRunning, false},
		{StateRunning, StatePermission, false},
		{StatePermission, StatePermission, false},
		{StatePermission, StateDone, false},
	}
	for _, c := range cases {
		if got := BannerStale(c.prev, c.next); got != c.want {
			t.Errorf("BannerStale(%s, %s) = %v, want %v", c.prev, c.next, got, c.want)
		}
	}
}

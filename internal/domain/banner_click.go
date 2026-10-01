package domain

import "strings"

// why: terminals that set no __CFBundleIdentifier still name themselves in TERM_PROGRAM.
var termProgramBundles = map[string]string{
	"Apple_Terminal": "com.apple.Terminal",
	"iTerm.app":      "com.googlecode.iterm2",
	"ghostty":        "com.mitchellh.ghostty",
	"WezTerm":        "com.github.wez.wezterm",
	"vscode":         "com.microsoft.VSCode",
	"WarpTerminal":   "dev.warp.Warp-Stable",
}

// why: a banner click brings this app to the front; empty when unknown, so no app is activated on a guess.
func TerminalBundle(bundleID, termProgram string) string {
	if id := strings.TrimSpace(bundleID); id != "" {
		return id
	}
	return termProgramBundles[termProgram]
}

// why: a banner left up after the session resumed points at nothing to do.
func BannerStale(prev, next AgentState) bool {
	switch prev {
	case StatePermission, StateWaiting, StateDone:
		return next == StateRunning || next == StateIdle
	}
	return false
}

package domain

import "strings"

var termProgramBundles = map[string]string{
	"Apple_Terminal": "com.apple.Terminal",
	"iTerm.app":      "com.googlecode.iterm2",
	"ghostty":        "com.mitchellh.ghostty",
	"WezTerm":        "com.github.wez.wezterm",
	"vscode":         "com.microsoft.VSCode",
	"WarpTerminal":   "dev.warp.Warp-Stable",
}

func TerminalBundle(bundleID, termProgram string) string {
	if id := strings.TrimSpace(bundleID); id != "" {
		return id
	}
	return termProgramBundles[termProgram]
}

func BannerStale(prev, next AgentState) bool {
	switch prev {
	case StatePermission, StateWaiting, StateDone:
		return next == StateRunning || next == StateIdle
	}
	return false
}

// Package notify shows macOS banners through osascript and tells whether a
// terminal app is in front.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

var (
	_ app.Notifier   = Osascript{}
	_ app.Foreground = Osascript{}
)

// Runner runs a command and returns its stdout.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

type Osascript struct {
	Run Runner
}

func New() Osascript { return Osascript{Run: execRunner} }

// why: title, body and sound reach AppleScript as argv, never spliced into the script, so quotes in a session name cannot break out of it.
var bannerScript = []string{
	"on run argv",
	"if item 3 of argv is \"\" then",
	"display notification (item 2 of argv) with title (item 1 of argv)",
	"else",
	"display notification (item 2 of argv) with title (item 1 of argv) sound name (item 3 of argv)",
	"end if",
	"end run",
}

func (o Osascript) Notify(ctx context.Context, b domain.Banner) error {
	args := make([]string, 0, 2*len(bannerScript)+3)
	for _, line := range bannerScript {
		args = append(args, "-e", line)
	}
	args = append(args, b.Title, b.Body, b.Sound)
	_, err := o.Run(ctx, "osascript", args...)
	return err
}

const frontmostScript = `tell application "System Events" to get name of first application process whose frontmost is true`

var terminalApps = map[string]bool{
	"Terminal": true, "iTerm2": true, "Ghostty": true, "WezTerm": true,
	"kitty": true, "Alacritty": true, "Hyper": true, "Warp": true,
}

// TerminalFrontmost is false when the query fails, so a banner is never
// suppressed on a guess.
func (o Osascript) TerminalFrontmost(ctx context.Context) bool {
	out, err := o.Run(ctx, "osascript", "-e", frontmostScript)
	return err == nil && terminalApps[strings.TrimSpace(string(out))]
}

// LoadSounds reads the sound name per event state from the file's "sounds"
// object, such as {"sounds":{"permission":"Glass"}}. A missing file means no
// sounds. Names for states that never notify are dropped.
func LoadSounds(path string) (map[domain.AgentState]string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Sounds map[string]string `json:"sounds"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	out := map[domain.AgentState]string{}
	for _, st := range []domain.AgentState{domain.StatePermission, domain.StateWaiting, domain.StateDone} {
		if name := cfg.Sounds[string(st)]; name != "" {
			out[st] = name
		}
	}
	return out, nil
}

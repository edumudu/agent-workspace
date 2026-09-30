package tmux

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/giovaniif/agent-workspace/internal/app"
)

// SendText pastes text through a tmux buffer so it arrives as one paste, not
// as keystrokes. With bracketedPaste the pane's app gets the paste markers
// (if it asked for them) and newlines stay newlines; without it, newlines
// become carriage returns, as Enter would send.
func (h *Host) SendText(ctx context.Context, pane app.PaneID, text string, bracketedPaste bool) error {
	buffer := fmt.Sprintf("agentws-%d-%d", os.Getpid(), h.bufferSeq.Add(1))
	if _, err := h.run(ctx, text, "load-buffer", "-b", buffer, "-"); err != nil {
		return err
	}
	args := []string{"paste-buffer", "-d", "-b", buffer, "-t", string(pane)}
	if bracketedPaste {
		args = append(args, "-p", "-r")
	}
	_, err := h.run(ctx, "", args...)
	return err
}

func (h *Host) SendKeys(ctx context.Context, pane app.PaneID, keys ...string) error {
	_, err := h.run(ctx, "", append([]string{"send-keys", "-t", string(pane)}, keys...)...)
	return err
}

// Capture returns the last lines lines of the pane, scrollback included,
// with wrapped lines joined and trailing blank lines dropped.
func (h *Host) Capture(ctx context.Context, pane app.PaneID, lines int) (string, error) {
	out, err := h.run(ctx, "", "capture-pane", "-p", "-J", "-t", string(pane), "-S", fmt.Sprintf("-%d", lines))
	if err != nil {
		return "", err
	}
	all := strings.Split(trimCapture(out), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n"), nil
}

func trimCapture(s string) string {
	rows := strings.Split(s, "\n")
	for len(rows) > 0 && strings.TrimSpace(rows[len(rows)-1]) == "" {
		rows = rows[:len(rows)-1]
	}
	return strings.Join(rows, "\n")
}

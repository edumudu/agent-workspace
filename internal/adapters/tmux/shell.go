package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/giovaniif/agent-workspace/internal/app"
)

const belowPaneIndex = "2"

// ShellHeight is the share of the main slot a shell split takes.
const ShellHeight = "35%"

const popupSessionPrefix = "agentws-popup-"

// BelowPane is the pane split below the agent pane, or "" if there is none.
func (h *Host) BelowPane(ctx context.Context, slot app.Slot) app.PaneID {
	// why: display-message falls back to another pane when the index does not exist, so list the panes instead.
	out, err := h.run(ctx, "", "list-panes", "-t", string(slot), "-F", "#{pane_index} #{pane_id}")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if index, id, ok := strings.Cut(strings.TrimSpace(line), " "); ok && index == belowPaneIndex {
			return app.PaneID(id)
		}
	}
	return ""
}

// ShowBelow splits pane below the agent pane, leaving focus where it was.
// A pane already there is parked first, alive.
func (h *Host) ShowBelow(ctx context.Context, pane app.PaneID, slot app.Slot) error {
	switch h.BelowPane(ctx, slot) {
	case pane:
		return nil
	case "":
	default:
		if err := h.HideBelow(ctx, slot); err != nil {
			return err
		}
	}
	// why: -d keeps focus where it was, so the sidebar keys still work after t.
	_, err := h.run(ctx, "", "join-pane", "-d", "-v", "-l", ShellHeight, "-s", string(pane), "-t", string(slot)+"."+slotPaneIndex)
	return err
}

// FocusBelow puts keyboard focus in the pane below the agent pane.
func (h *Host) FocusBelow(ctx context.Context, slot app.Slot) error {
	below := h.BelowPane(ctx, slot)
	if below == "" {
		return errors.New("no shell is shown below the agent pane")
	}
	_, err := h.run(ctx, "", "select-pane", "-t", string(below))
	return err
}

// HideBelow moves the pane below the agent pane back to a window of its own,
// still running.
func (h *Host) HideBelow(ctx context.Context, slot app.Slot) error {
	below := h.BelowPane(ctx, slot)
	if below == "" {
		return nil
	}
	_, err := h.run(ctx, "", "break-pane", "-d", "-s", string(below), "-n", "pane-parked")
	return err
}

// Popup opens the pane in a popup over an attached client. The popup is a
// second client on a grouped session showing the pane's window, so the pane
// keeps running after the popup closes with M-t or the pane's exit.
func (h *Host) Popup(ctx context.Context, pane app.PaneID) error {
	window, err := h.parkedWindow(ctx, pane)
	if err != nil {
		return err
	}
	client, err := h.popupClient(ctx)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%s%d-%d", popupSessionPrefix, os.Getpid(), h.bufferSeq.Add(1))
	inner := strings.Join([]string{
		"env", "-u", "TMUX", "tmux", "-L", shellQuote(h.socket), "-f", shellQuote(h.configPath),
		"new-session", "-t", sessionName, "-s", name,
		`\;`, "select-window", "-t", window,
		`\;`, "set-option", "destroy-unattached", "on",
	}, " ")
	// why: display-popup returns only when the popup closes, and the daemon must not wait for a person.
	cmd := exec.Command("tmux", "-L", h.socket, "-f", h.configPath, "display-popup", "-c", client, "-E", "-w", "80%", "-h", "80%", inner)
	cmd.Env = withoutTmuxEnv(os.Environ())
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (h *Host) parkedWindow(ctx context.Context, pane app.PaneID) (string, error) {
	out, err := h.run(ctx, "", "display-message", "-p", "-t", string(pane), "#{window_id} #{window_panes}")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return "", &tmuxError{args: []string{"display-message"}, stderr: "unexpected output " + out}
	}
	if fields[1] == "1" {
		return fields[0], nil
	}
	parked, err := h.run(ctx, "", "break-pane", "-d", "-P", "-F", "#{window_id}", "-s", string(pane), "-n", "pane-parked")
	return strings.TrimSpace(parked), err
}

func (h *Host) popupClient(ctx context.Context) (string, error) {
	out, err := h.run(ctx, "", "list-clients", "-F", "#{client_name} #{session_name}")
	if err != nil && !isServerGone(err) {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && !strings.HasPrefix(fields[1], popupSessionPrefix) {
			return fields[0], nil
		}
	}
	return "", errors.New("no terminal is attached to the agentws tmux server")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

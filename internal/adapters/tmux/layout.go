package tmux

import (
	"context"
	"strconv"
	"strings"

	"github.com/giovaniif/agent-workspace/internal/app"
)

const slotPaneIndex = "1"

// SidebarWidth is the TUI pane's width in columns; the main slot gets the rest.
const SidebarWidth = 48

// OpenClient creates the client window: the TUI's pane on the left and an
// empty main slot on the right. The returned Slot is the window's ID.
func (h *Host) OpenClient(ctx context.Context, name string, tui app.PaneSpec) (app.Slot, error) {
	id, err := h.newWindow(ctx, "client-"+name, tui)
	if err != nil {
		return "", err
	}
	slot := app.Slot(id.window)
	if err := h.addSlotPane(ctx, slot); err != nil {
		return "", err
	}
	if err := h.pinSidebar(ctx, slot, strconv.Itoa(SidebarWidth)); err != nil {
		return "", err
	}
	return slot, nil
}

// ReviewWidth is the sidebar pane's share of the window while the review is
// open; the agent pane keeps the rest.
const ReviewWidth = "75%"

// WidenSidebar gives the sidebar pane ReviewWidth, or puts it back to
// SidebarWidth, and keeps that width across window resizes.
func (h *Host) WidenSidebar(ctx context.Context, slot app.Slot, wide bool) error {
	width := strconv.Itoa(SidebarWidth)
	if wide {
		width = ReviewWidth
	}
	return h.pinSidebar(ctx, slot, width)
}

func (h *Host) pinSidebar(ctx context.Context, slot app.Slot, width string) error {
	target := string(slot) + ".0"
	if _, err := h.run(ctx, "", "resize-pane", "-t", target, "-x", width); err != nil {
		return err
	}
	// why: tmux spreads a window resize over both panes; the hook puts the sidebar back to its width.
	_, err := h.run(ctx, "", "set-hook", "-w", "-t", string(slot), "window-resized", "resize-pane -t "+target+" -x "+width)
	return err
}

func (h *Host) addSlotPane(ctx context.Context, slot app.Slot) error {
	_, err := h.run(ctx, "", "split-window", "-h", "-d", "-l", "70%", "-t", string(slot)+".0", emptyState)
	return err
}

// EnsureSlot gives the slot an empty-state pane if its pane is gone, such as
// after the pane shown in it was killed.
func (h *Host) EnsureSlot(ctx context.Context, slot app.Slot) error {
	count, err := h.paneCount(ctx, slot)
	if err != nil || count != 1 {
		return err
	}
	return h.addSlotPane(ctx, slot)
}

// Show swaps pane into the slot; the pane that was there goes back to the
// window the shown pane came from. The common path is a single tmux call.
func (h *Host) Show(ctx context.Context, pane app.PaneID, slot app.Slot) error {
	err := h.swapIntoSlot(ctx, pane, slot)
	if err == nil {
		return nil
	}
	count, countErr := h.paneCount(ctx, slot)
	if countErr != nil || count != 1 {
		return err
	}
	if addErr := h.addSlotPane(ctx, slot); addErr != nil {
		return addErr
	}
	return h.swapIntoSlot(ctx, pane, slot)
}

func (h *Host) swapIntoSlot(ctx context.Context, pane app.PaneID, slot app.Slot) error {
	_, err := h.run(ctx, "", "swap-pane", "-d", "-s", string(pane), "-t", string(slot)+"."+slotPaneIndex)
	return err
}

// SlotHasPane is false once the pane shown in the slot has exited: tmux drops
// it and the sidebar is the only pane left. An unreadable window counts as
// having one, so a tmux hiccup never triggers a refill.
func (h *Host) SlotHasPane(ctx context.Context, slot app.Slot) bool {
	count, err := h.paneCount(ctx, slot)
	return err != nil || count > 1
}

func (h *Host) paneCount(ctx context.Context, slot app.Slot) (int, error) {
	out, err := h.run(ctx, "", "list-panes", "-t", string(slot), "-F", "#{pane_id}")
	if err != nil {
		return 0, err
	}
	return len(strings.Fields(out)), nil
}

func (h *Host) ClientOpen(ctx context.Context, slot app.Slot) bool {
	out, err := h.run(ctx, "", "display-message", "-p", "-t", string(slot), "#{window_id}")
	return err == nil && strings.TrimSpace(out) == string(slot)
}

// FocusSlot makes the slot's pane the active one, so keys go to the agent.
func (h *Host) FocusSlot(ctx context.Context, slot app.Slot) error {
	_, err := h.run(ctx, "", "select-pane", "-t", string(slot)+"."+slotPaneIndex)
	return err
}

// AttachCommand is the argv that attaches a terminal to the client window.
// It always ends with "attach-session -t <slot>".
func (h *Host) AttachCommand(slot app.Slot) []string {
	return []string{"tmux", "-L", h.socket, "-f", h.configPath, "attach-session", "-t", string(slot)}
}

// ShownIn returns the pane currently in the slot, or "" if there is none.
func (h *Host) ShownIn(ctx context.Context, slot app.Slot) app.PaneID {
	out, err := h.run(ctx, "", "display-message", "-p", "-t", string(slot)+"."+slotPaneIndex, "#{pane_id}")
	if err != nil {
		return ""
	}
	return app.PaneID(strings.TrimSpace(out))
}

// Detach detaches every terminal attached to slot's session; the layout and
// the sessions in it keep running for the next attach.
func (h *Host) Detach(ctx context.Context, slot app.Slot) error {
	session, err := h.run(ctx, "", "display-message", "-p", "-t", string(slot), "#{session_name}")
	if err != nil {
		return err
	}
	_, err = h.run(ctx, "", "detach-client", "-s", strings.TrimSpace(session))
	return err
}

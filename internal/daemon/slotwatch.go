package daemon

import (
	"context"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

const slotWatchEvery = 2 * time.Second

func WithSlotWatch(every time.Duration) Option {
	return func(d *Daemon) { d.clients.watchEvery = every }
}

// why: covers an agent that exits by itself: tmux drops its pane, so the
// slot is gone without session.end.
func (d *Daemon) watchMainSlot(ctx context.Context) {
	tick, stop := ticker(d.clients.watchEvery)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			d.refillLostSlot(ctx)
		}
	}
}

func (d *Daemon) refillLostSlot(ctx context.Context) {
	d.clients.mu.Lock()
	host, slot := d.clients.host, d.clients.slot
	d.clients.mu.Unlock()
	if host == nil || slot == "" {
		return
	}
	// why: read before the pane check, so a session that takes the slot during it is not taken for the one that lost it.
	inView, found, ok := d.sessionInView()
	if !ok || host.SlotHasPane(ctx, slot) {
		return
	}
	if !found {
		d.refillMain(domain.Session{}, false)
		return
	}
	if d.restoreLiveAgent(ctx, inView, slot) {
		return
	}
	_, _, _ = d.endAndRefill(inView)
}

// why: holds clients.mu, which focusSession holds from showing a pane until it marks the session focused,
// so a session focused meanwhile is seen and never covered. An editor or shell shown in the slot leaves
// it empty when it quits, while the agent it replaced is still alive. Returns false only when the agent's
// pane is gone and the session should end.
func (d *Daemon) restoreLiveAgent(ctx context.Context, inView domain.Session, slot app.Slot) bool {
	d.clients.mu.Lock()
	defer d.clients.mu.Unlock()
	if now, stillFound, ok := d.sessionInView(); !ok || !stillFound || now.ID != inView.ID || now.Pane != inView.Pane {
		return true
	}
	if d.hs.host == nil || inView.Pane == "" {
		return false
	}
	alive, err := d.hs.host.Alive(ctx, app.PaneID(inView.Pane))
	if err != nil {
		return true
	}
	if alive {
		_ = d.hs.host.Show(ctx, app.PaneID(inView.Pane), slot)
	}
	return alive
}

func (d *Daemon) sessionInView() (session domain.Session, found, ok bool) {
	ok = d.query(func(s *state) {
		for _, cur := range sorted(s.sessions) {
			if cur.Focused {
				session, found = cur, true
			}
		}
	})
	return session, found, ok
}

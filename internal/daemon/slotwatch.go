package daemon

import (
	"context"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

const slotWatchEvery = 2 * time.Second

// WithSlotWatch sets how often the daemon checks that the main slot still has
// a pane. Zero, the default, turns the check off.
func WithSlotWatch(every time.Duration) Option {
	return func(d *Daemon) { d.clients.watchEvery = every }
}

// watchMainSlot covers an agent that exits by itself: tmux drops its pane, so
// the slot is gone without session.end. It is a worker, never the event loop.
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
	if host == nil || slot == "" || host.SlotHasPane(ctx, slot) {
		return
	}
	var inView domain.Session
	var found bool
	if !d.query(func(s *state) {
		for _, cur := range sorted(s.sessions) {
			if cur.Focused {
				inView, found = cur, true
			}
		}
	}) {
		return
	}
	if !found {
		d.refillMain(domain.Session{}, false)
		return
	}
	_, _, _ = d.endAndRefill(inView)
}

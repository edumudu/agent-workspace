package daemon

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// ClientHost is the part of the terminal host that manages the client layout:
// one window with the TUI on the left and the main slot on the right.
type ClientHost interface {
	OpenClient(ctx context.Context, name string, tui app.PaneSpec) (app.Slot, error)
	ClientOpen(ctx context.Context, slot app.Slot) bool
	AttachCommand(slot app.Slot) []string
	FocusSlot(ctx context.Context, slot app.Slot) error
}

const clientName = "main"

const clientTimeout = 5 * time.Second

type clients struct {
	mu   sync.Mutex
	host ClientHost
	slot app.Slot
}

// SetClientHost enables client.open and client.focus_main. Call it before Serve.
func (d *Daemon) SetClientHost(h ClientHost) {
	d.clients.mu.Lock()
	defer d.clients.mu.Unlock()
	d.clients.host = h
}

// why: these calls run tmux, so they stay on the connection goroutine and never enter the loop.
func (d *Daemon) dispatchClient(req rpc.Request) *rpc.Response {
	d.clients.mu.Lock()
	defer d.clients.mu.Unlock()
	h := d.clients.host
	if h == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "this daemon has no terminal host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()
	switch req.Method {
	case rpc.MethodOpenClient:
		var p rpc.OpenClientParams
		if err := json.Unmarshal(req.Params, &p); err != nil || len(p.Command) == 0 {
			return errorResponse(req.ID, rpc.CodeBadRequest, "client.open needs a command")
		}
		if d.clients.slot == "" || !h.ClientOpen(ctx, d.clients.slot) {
			slot, err := h.OpenClient(ctx, clientName, app.PaneSpec{Name: clientName, Command: p.Command, Env: p.Env})
			if err != nil {
				return errorResponse(req.ID, rpc.CodeFailed, err.Error())
			}
			d.clients.slot = slot
		}
		return result(req.ID, rpc.OpenClient{Slot: string(d.clients.slot), Attach: h.AttachCommand(d.clients.slot)})
	default:
		if d.clients.slot == "" {
			return errorResponse(req.ID, rpc.CodeFailed, "no client layout is open")
		}
		if err := h.FocusSlot(ctx, d.clients.slot); err != nil {
			return errorResponse(req.ID, rpc.CodeFailed, err.Error())
		}
		return result(req.ID, struct{}{})
	}
}

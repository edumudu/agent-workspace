package daemon

import (
	"context"
	"encoding/json"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const sendSwitchTimeout = 5 * time.Second

func (d *Daemon) switchSession(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.SwitchParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, rpc.CodeBadRequest, "switch params: "+err.Error()), true
	}
	if (p.Kind != domain.SwitchModel && p.Kind != domain.SwitchEffort) || p.Value == "" {
		return errorResponse(req.ID, rpc.CodeBadRequest, "switch needs a kind (model or effort) and a value"), true
	}
	if d.hs.host == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "no terminal host"), true
	}
	var resp *rpc.Response
	ok := d.query(func(s *state) {
		session, found := s.sessions[p.SessionID]
		if !found {
			resp = errorResponse(req.ID, rpc.CodeNotFound, "no session "+p.SessionID)
			return
		}
		next, toSend := session.RequestSwitch(p.Kind, p.Value).Dispatch(time.Now())
		s.emit(SessionChanged{Session: next})
		s.sendSwitches(next, toSend)
		resp = result(req.ID, next)
	})
	return resp, ok
}

// sendSwitches types the switches into the session's pane on a worker, since
// tmux never runs on the loop. Workers take turns, so two switches for one
// pane are typed in the order they were dispatched.
func (d *Daemon) sendSwitches(session domain.Session, sws []domain.Switch) {
	if len(sws) == 0 {
		return
	}
	go func() {
		d.hs.sendMu.Lock()
		defer d.hs.sendMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), sendSwitchTimeout)
		defer cancel()
		err := app.SendSwitches(ctx, d.hs.host, app.PaneID(session.Pane), session.Harness, sws, app.PasteSettle)
		if err == nil {
			return
		}
		d.query(func(s *state) {
			if current, ok := s.sessions[session.ID]; ok {
				s.emit(SessionChanged{Session: current.SwitchFailed(sws)})
			}
		})
	}()
}

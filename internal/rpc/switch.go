package rpc

import (
	"context"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// MethodSwitch asks the daemon to change a session's model or effort. The
// daemon types the harness's command into the pane once the agent is between
// tools; the result is the session with the switch queued or sent.
const MethodSwitch = "session.switch"

type SwitchParams struct {
	SessionID string            `json:"session_id"`
	Kind      domain.SwitchKind `json:"kind"`
	Value     string            `json:"value"`
}

func (c *Client) SwitchSession(ctx context.Context, sessionID string, kind domain.SwitchKind, value string) (domain.Session, error) {
	var out domain.Session
	err := c.Call(ctx, MethodSwitch, SwitchParams{SessionID: sessionID, Kind: kind, Value: value}, &out)
	return out, err
}

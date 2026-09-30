package app

import (
	"context"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// PasteSettle is how long to wait between pasting a command and pressing
// Enter, so the harness has taken the whole paste before it sees the key.
const PasteSettle = 150 * time.Millisecond

// SendSwitches types each switch's slash command into the pane as one
// bracketed paste followed by Enter, and stops at the first failure.
func SendSwitches(ctx context.Context, host TerminalHost, pane PaneID, h domain.Harness, sws []domain.Switch, settle time.Duration) error {
	for _, sw := range sws {
		if err := host.SendText(ctx, pane, domain.SwitchCommand(h, sw), true); err != nil {
			return err
		}
		select {
		case <-time.After(settle):
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := host.SendKeys(ctx, pane, "Enter"); err != nil {
			return err
		}
	}
	return nil
}

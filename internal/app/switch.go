package app

import (
	"context"
	"errors"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// PasteSettle is how long to wait between pasting a command and pressing
// Enter, so the harness has taken the whole paste before it sees the key.
const PasteSettle = 150 * time.Millisecond

const (
	// codexPickerSteps covers the model popup, the "All models" list and the
	// reasoning level popup, with one spare for a slow redraw.
	codexPickerSteps = 4
	codexPickerLines = 40
)

var errNoCodexPicker = errors.New("the Codex /model picker did not open")

// SendSwitches types each switch into the session's pane and stops at the
// first failure. Claude takes the slash command as one bracketed paste
// followed by Enter; Codex gets `/model` and then the keys that walk its picker.
func SendSwitches(ctx context.Context, host TerminalHost, s domain.Session, sws []domain.Switch, settle time.Duration) error {
	pane := PaneID(s.Pane)
	for _, sw := range sws {
		if err := host.SendText(ctx, pane, domain.SwitchCommand(s.Harness, sw), true); err != nil {
			return err
		}
		if err := wait(ctx, settle); err != nil {
			return err
		}
		if err := host.SendKeys(ctx, pane, "Enter"); err != nil {
			return err
		}
		if s.Harness != domain.HarnessCodex {
			continue
		}
		if err := walkCodexPicker(ctx, host, pane, s, sw, settle); err != nil {
			return err
		}
		if sw.Kind == domain.SwitchModel {
			s.Model = sw.Value
		} else {
			s.Effort = sw.Value
		}
	}
	return nil
}

// walkCodexPicker reads the pane after each step and presses the keys the
// domain picks, until no popup is left. Anything it cannot place closes the
// picker with Escape, so no half-made choice is left open.
func walkCodexPicker(ctx context.Context, host TerminalHost, pane PaneID, s domain.Session, sw domain.Switch, settle time.Duration) error {
	for step := range codexPickerSteps {
		if err := wait(ctx, settle); err != nil {
			return err
		}
		screen, err := host.Capture(ctx, pane, codexPickerLines)
		if err != nil {
			return err
		}
		picker, open := domain.ParsePicker(screen)
		if !open && step == 0 {
			return errNoCodexPicker
		}
		if !open {
			return nil
		}
		keys, err := domain.CodexPickerKeys(picker, sw, s.Model, s.Effort)
		if err != nil {
			return errors.Join(err, host.SendKeys(ctx, pane, "Escape"))
		}
		if err := host.SendKeys(ctx, pane, keys...); err != nil {
			return err
		}
	}
	return errors.Join(errors.New("the Codex picker is still open"), host.SendKeys(ctx, pane, "Escape"))
}

func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

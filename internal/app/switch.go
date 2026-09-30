package app

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// PasteSettle is how long to wait between pasting a command and pressing
// Enter, so the harness has taken the whole paste before it sees the key.
const PasteSettle = 150 * time.Millisecond

const (
	// codexPickerSteps covers the quick model popup, the "All models" list,
	// the reasoning level popup and one spare.
	codexPickerSteps = 4
	// codexPickerPolls bounds how many captures one step waits for Codex to
	// redraw, one settle apart.
	codexPickerPolls = 20
	codexPickerLines = 40
	escapeTimeout    = time.Second
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

// walkCodexPicker reads the pane and presses the keys the domain picks,
// until no popup is left. After each press it waits for the picker to change,
// since tmux returns before Codex redraws and a stale screen would steer the
// next keys into the wrong popup. Anything it cannot place closes the picker
// with Escape, so no half-made choice is left open for later input.
func walkCodexPicker(ctx context.Context, host TerminalHost, pane PaneID, s domain.Session, sw domain.Switch, settle time.Duration) error {
	var last *domain.Picker
	for range codexPickerSteps {
		picker, open, err := nextPicker(ctx, host, pane, last, settle)
		switch {
		case err != nil:
			return errors.Join(err, escape(host, pane))
		case !open && last == nil:
			return errNoCodexPicker
		case !open:
			return nil
		}
		keys, err := domain.CodexPickerKeys(picker, sw, s.Model, s.Effort)
		if err != nil {
			return errors.Join(err, escape(host, pane))
		}
		if err := host.SendKeys(ctx, pane, keys...); err != nil {
			return errors.Join(err, escape(host, pane))
		}
		last = &picker
	}
	return errors.Join(errors.New("the Codex picker is still open"), escape(host, pane))
}

// nextPicker polls until the pane shows a picker other than last, or none
// once one was acted on. Before the first step it waits for one to open.
func nextPicker(ctx context.Context, host TerminalHost, pane PaneID, last *domain.Picker, settle time.Duration) (domain.Picker, bool, error) {
	for range codexPickerPolls {
		if err := wait(ctx, settle); err != nil {
			return domain.Picker{}, false, err
		}
		screen, err := host.Capture(ctx, pane, codexPickerLines)
		if err != nil {
			return domain.Picker{}, false, err
		}
		picker, open := domain.ParsePicker(screen)
		switch {
		case open && (last == nil || !reflect.DeepEqual(picker, *last)):
			return picker, true, nil
		case !open && last != nil:
			return domain.Picker{}, false, nil
		}
	}
	if last == nil {
		return domain.Picker{}, false, nil
	}
	return domain.Picker{}, false, errors.New("the Codex picker did not redraw")
}

// escape uses its own context: the caller's may be the one that ran out.
func escape(host TerminalHost, pane PaneID) error {
	ctx, cancel := context.WithTimeout(context.Background(), escapeTimeout)
	defer cancel()
	return host.SendKeys(ctx, pane, "Escape")
}

func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

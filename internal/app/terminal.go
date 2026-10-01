package app

import "context"

type PaneID string

// Slot names a client's main area, the one place a pane is shown.
type Slot string

type PaneSpec struct {
	Name    string
	Dir     string
	Command []string
	Env     map[string]string
}

type PaneInfo struct {
	ID    PaneID
	Alive bool
}

type TerminalHost interface {
	Create(ctx context.Context, spec PaneSpec) (PaneID, error)
	Kill(ctx context.Context, pane PaneID) error
	List(ctx context.Context) ([]PaneInfo, error)
	OpenClient(ctx context.Context, name string, tui PaneSpec) (Slot, error)
	Show(ctx context.Context, pane PaneID, slot Slot) error
	SendText(ctx context.Context, pane PaneID, text string, bracketedPaste bool) error
	SendKeys(ctx context.Context, pane PaneID, keys ...string) error
	Capture(ctx context.Context, pane PaneID, lines int) (string, error)
	Alive(ctx context.Context, pane PaneID) (bool, error)
	// SetTitle sets the text on the pane's top border; empty clears it.
	SetTitle(ctx context.Context, pane PaneID, title string) error
}

// Editor evaluates an expression in a running nvim, found by the socket it
// was started with `--listen`.
type Editor interface {
	Eval(ctx context.Context, socket, expr string) error
}

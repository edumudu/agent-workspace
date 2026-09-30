package tui_test

import "context"

type fakeFocuser struct{ calls int }

func (f *fakeFocuser) FocusMain(context.Context) error {
	f.calls++
	return nil
}

package tui_test

import "context"

type fakeFocuser struct{ calls int }

func (f *fakeFocuser) FocusMain(context.Context) error {
	f.calls++
	return nil
}

type fakeAttender struct {
	muted   []mute
	focused []string
}

type mute struct {
	id    string
	muted bool
}

func (f *fakeAttender) MuteSession(_ context.Context, id string, muted bool) error {
	f.muted = append(f.muted, mute{id, muted})
	return nil
}

func (f *fakeAttender) FocusSession(_ context.Context, id string) error {
	f.focused = append(f.focused, id)
	return nil
}

type fakeKiller struct {
	killed [][]int
	err    error
}

func (f *fakeKiller) KillPorts(_ context.Context, pgids []int) ([]int, error) {
	f.killed = append(f.killed, pgids)
	return pgids, f.err
}

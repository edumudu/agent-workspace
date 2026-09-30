package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const terminalCallTimeout = 5 * time.Second

// reviewWorktree is the worktree the review last narrowed to for the
// selected session; empty lets the daemon pick.
func (m Model) reviewWorktree() string {
	if m.rv.session == m.selected {
		return m.rv.worktree
	}
	return ""
}

func (m Model) callDaemon(method string, params any) tea.Cmd {
	c := m.opts.Calls
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), terminalCallTimeout)
		defer cancel()
		if err := c.Call(ctx, method, params, nil); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m Model) toggleShell(popup bool) tea.Cmd {
	if m.opts.Calls == nil || m.selected == "" {
		return nil
	}
	return m.callDaemon(rpc.MethodShellToggle, rpc.ShellParams{Session: m.selected, Worktree: m.reviewWorktree(), Popup: popup})
}

func (m Model) toggleNvim() tea.Cmd {
	if m.opts.Calls == nil || m.selected == "" {
		return nil
	}
	return m.callDaemon(rpc.MethodNvimToggle, rpc.NvimParams{Session: m.selected, Worktree: m.reviewWorktree()})
}

// openInNvim closes the review, so nvim gets the room the review took, and
// then opens the diff's top line in the session's nvim.
func (m Model) openInNvim() (tea.Model, tea.Cmd) {
	f, ok := m.rv.current()
	if !ok || m.opts.Calls == nil {
		return m, nil
	}
	m.rv.open = false
	r := m.opts.Review
	open := m.callDaemon(rpc.MethodNvimOpen, rpc.NvimParams{
		Session: m.rv.session, Worktree: f.wt.ID, Path: f.file.Path, Line: m.topLine(f),
	})
	return m, func() tea.Msg {
		if msg := layout(r, false)(); msg != nil {
			return msg
		}
		return open()
	}
}

// topLine is the new-side line number of the first row at or below the top of
// the diff view that has one, so a deleted line opens the line after it.
func (m Model) topLine(f reviewFile) int {
	rows := f.unified
	if m.rv.split {
		rows = f.split
	}
	for i := max(m.rv.scroll, 0); i < len(rows); i++ {
		if n := newLine(rows[i]); n > 0 {
			return n
		}
	}
	for i := min(m.rv.scroll, len(rows)) - 1; i >= 0; i-- {
		if n := newLine(rows[i]); n > 0 {
			return n
		}
	}
	return 1
}

func newLine(r diffRow) int {
	switch {
	case r.one != nil:
		return r.one.line.New
	case r.right != nil:
		return r.right.line.New
	}
	return 0
}

func (m Model) draftCount(session string) int {
	n := 0
	for _, c := range m.comments {
		if c.Session == session {
			n++
		}
	}
	return n
}

func (m *Model) putComment(c domain.DraftComment) { m.comments[c.ID] = c }

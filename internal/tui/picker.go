package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// Switcher asks the daemon to change a session's model or effort;
// *rpc.Client is one.
type Switcher interface {
	SwitchSession(ctx context.Context, sessionID string, kind domain.SwitchKind, value string) (domain.Session, error)
}

// picker is the open model or effort list for one session.
type picker struct {
	sessionID string
	harness   domain.Harness
	kind      domain.SwitchKind
	choices   []string
	cursor    int
}

func (m Model) openPicker(kind domain.SwitchKind) Model {
	i := m.index(m.selected)
	if i < 0 || m.opts.Switch == nil {
		return m
	}
	s := m.entries[i].session
	m.picker = &picker{sessionID: s.ID, harness: s.Harness, kind: kind, choices: domain.SwitchChoices(s.Harness, kind)}
	return m
}

func (m Model) pickerKey(k string) (tea.Model, tea.Cmd) {
	p := *m.picker
	switch k {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.picker = nil
	case "j", "down":
		p.cursor = min(p.cursor+1, len(p.choices)-1)
		m.picker = &p
	case "k", "up":
		p.cursor = max(p.cursor-1, 0)
		m.picker = &p
	case "enter":
		return m.applyChoice(p, p.cursor)
	default:
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' && int(k[0]-'1') < len(p.choices) {
			return m.applyChoice(p, int(k[0]-'1'))
		}
	}
	return m, nil
}

func (m Model) applyChoice(p picker, i int) (tea.Model, tea.Cmd) {
	m.picker = nil
	sw := m.opts.Switch
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := sw.SwitchSession(ctx, p.sessionID, p.kind, p.choices[i]); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m Model) pickerLines() []string {
	p := m.picker
	s := m.styles
	title := " MODEL"
	if p.kind == domain.SwitchEffort {
		title = " EFFORT"
	}
	out := []string{"", m.line(false, []piece{{s.header, title}, {s.sub, fmt.Sprintf(" · %s", p.harness)}}, nil)}
	for i, c := range p.choices {
		label := piece{s.text, fmt.Sprintf(" %d  %s", i+1, c)}
		if i == p.cursor {
			label = piece{s.bold, fmt.Sprintf("▌%d  %s", i+1, c)}
		}
		out = append(out, m.line(i == p.cursor, []piece{label}, nil))
	}
	out = append(out, "", m.line(false, []piece{{s.dim, " j/k or 1-9 pick · ⏎ apply · esc cancel"}}, nil))
	return out
}

// switchMarks are the sidebar pieces for a session's unconfirmed switches: the
// values on their way, and a warning when a harness did not confirm one.
func (m Model) switchMarks(x domain.Session) []piece {
	var marks []piece
	for _, sw := range x.Switches {
		marks = append(marks, piece{m.styles.peach, "→ " + sw.Value + " "})
	}
	if x.SwitchWarning {
		marks = append(marks, piece{m.styles.need, "! "})
	}
	return marks
}

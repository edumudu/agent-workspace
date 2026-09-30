package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// renamePrompt is the open rename line for one session.
type renamePrompt struct {
	id   string
	text string
}

func (m Model) askRename() Model {
	i := m.index(m.selected)
	if i < 0 || m.opts.Calls == nil {
		return m
	}
	e := m.entries[i]
	m.renaming = &renamePrompt{id: e.session.ID, text: domain.NameFor(e.task, entryPRs(e))}
	return m
}

func entryPRs(e entry) []domain.PullRequest {
	var prs []domain.PullRequest
	for _, w := range e.worktrees {
		if w.PR != nil {
			prs = append(prs, *w.PR)
		}
	}
	return prs
}

func (m Model) renameKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := *m.renaming
	m.renaming = &p
	switch msg.String() {
	case "esc", "ctrl+c":
		m.renaming = nil
	case "enter":
		m.renaming = nil
		if name := strings.TrimSpace(p.text); name != "" {
			return m, m.call(rpc.MethodSessionRename, rpc.SessionRenameParams{ID: p.id, Name: name})
		}
	case "ctrl+u":
		p.text = ""
	case "backspace":
		if r := []rune(p.text); len(r) > 0 {
			p.text = string(r[:len(r)-1])
		}
	default:
		p.text += strings.ReplaceAll(msg.Text, "\n", " ")
	}
	return m, nil
}

func (m Model) renamePaste(s string) Model {
	p := *m.renaming
	p.text += strings.ReplaceAll(s, "\n", " ")
	m.renaming = &p
	return m
}

func (m Model) unpin() tea.Cmd {
	i := m.index(m.selected)
	if i < 0 || m.entries[i].task.PinnedName == "" {
		return nil
	}
	return m.call(rpc.MethodSessionUnpin, rpc.SessionRef{ID: m.selected})
}

// renameLeft is the status line while renaming. It shows the end of a text
// too long for the row, where the cursor is.
func (m Model) renameLeft() []piece {
	s := m.styles
	room := max(m.width-len(" RENAME ")-3, 1)
	text := []rune(m.renaming.text)
	if len(text) > room {
		text = text[len(text)-room:]
	}
	return []piece{{s.badge, " RENAME "}, {s.text, " " + string(text) + "▏"}}
}

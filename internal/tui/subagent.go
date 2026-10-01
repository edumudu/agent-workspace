package tui

import (
	"fmt"
	"strings"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

const maxSubagentRows = 8

func (m *Model) putSubagent(sub domain.Subagent) {
	list := m.subagents[sub.SessionID]
	for i, s := range list {
		if s.ID == sub.ID {
			list[i] = sub
			return
		}
	}
	m.subagents[sub.SessionID] = append(list, sub)
}

func (m Model) subagentLines(sessionID string, sel bool) []string {
	nodes := domain.SubagentTree(m.subagents[sessionID])
	if len(nodes) == 0 {
		return nil
	}
	s := m.styles
	bar := piece{s.text, " "}
	if sel {
		bar = piece{s.bar, "▌"}
	}
	var out []string
	for _, n := range nodes[:min(len(nodes), maxSubagentRows)] {
		label := cleanText(n.Type)
		if label == "" {
			label = cleanText(n.ID)
		}
		left := []piece{bar, {s.dim, "     " + strings.Repeat("  ", n.Depth) + "⤷ "}, {s.text, label}}
		if n.Summary != "" {
			left = append(left, piece{s.sub, " · " + cleanText(n.Summary)})
		}
		glyph := piece{s.green, "✓"}
		if n.State == domain.SubagentRunning {
			glyph = piece{s.blue, spinner[m.frame%len(spinner)]}
		}
		out = append(out, m.line(sel, left, []piece{glyph, {s.text, "   "}}))
	}
	if extra := len(nodes) - maxSubagentRows; extra > 0 {
		out = append(out, m.line(sel, []piece{bar, {s.dim, fmt.Sprintf("     … %d more", extra)}}, nil))
	}
	return out
}

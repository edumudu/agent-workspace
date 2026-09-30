package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// diskPanelsSideBySideFrom is the narrowest screen that puts the disk,
// cleanup and deps panels in one row, as in the mockup; below it they stack.
const diskPanelsSideBySideFrom = 140

const diskBarWidth = 40

// diskHeader is the three panels above the table: the disk bar with its
// legend, the auto-cleanup rules, and the shared deps store.
func (m Model) diskHeader() []string {
	panels := [][][]piece{m.diskPanel(), m.cleanupPanel()}
	if p := m.depsPanel(); p != nil {
		panels = append(panels, p)
	}
	out := []string{""}
	if m.width < diskPanelsSideBySideFrom {
		for _, p := range panels {
			for _, row := range p {
				out = append(out, m.line(false, row, nil))
			}
			out = append(out, "")
		}
		return out
	}
	widths := []int{diskBarWidth + 14, 42, 0}
	rows := 0
	for _, p := range panels {
		rows = max(rows, len(p))
	}
	for i := range rows {
		var line []piece
		for j, p := range panels {
			var cellPieces []piece
			if i < len(p) {
				cellPieces = p[i]
			}
			if j < len(panels)-1 {
				cellPieces = padCell(widths[j], cellPieces)
			}
			line = append(line, cellPieces...)
		}
		out = append(out, m.line(false, line, nil))
	}
	return append(out, "")
}

func padCell(width int, ps []piece) []piece {
	used := 0
	for _, p := range ps {
		used += ansi.StringWidth(p.s)
	}
	return append(ps, piece{lipgloss.NewStyle(), strings.Repeat(" ", max(width-used, 0))})
}

func (m Model) diskPanel() [][]piece {
	s := m.styles
	v := m.dk.view
	rows := m.diskRows()
	total, totalPending := domain.TotalSize(rows)
	reclaim, reclaimPending := domain.Reclaimable(rows)
	title := []piece{{s.bold, " Disk"}}
	if v.Total > 0 {
		free := fmt.Sprintf("%s free of %s", formatBytes(int64(v.Free)), formatBytes(int64(v.Total)))
		title = append(title, piece{s.sub, strings.Repeat(" ", max(diskBarWidth-5-len(free), 2)) + free})
	}
	legend := []piece{
		{s.blue, " ■"}, {s.sub, " worktrees " + totalText(total, totalPending)},
		{s.peach, "  ■"}, {s.sub, " reclaimable " + totalText(reclaim, reclaimPending)},
		{s.dim, "  ■"}, {s.sub, " other"},
	}
	return [][]piece{title, m.diskBar(total, reclaim), legend}
}

// diskBar splits the volume into kept worktrees, reclaimable ones, other
// used space and free space, each a share of diskBarWidth cells.
func (m Model) diskBar(worktrees, reclaim int64) []piece {
	s := m.styles
	v := m.dk.view
	if v.Total == 0 {
		return []piece{{s.dim, " " + strings.Repeat("░", diskBarWidth)}}
	}
	cells := func(n int64) int {
		return int(max(n, 0) * diskBarWidth / int64(v.Total))
	}
	used := int64(v.Total - v.Free)
	kept := cells(worktrees - reclaim)
	gone := cells(reclaim)
	other := max(cells(used)-kept-gone, 0)
	free := max(diskBarWidth-kept-gone-other, 0)
	return []piece{
		{s.text, " "},
		{s.blue, strings.Repeat("█", kept)},
		{s.peach, strings.Repeat("█", gone)},
		{s.dim, strings.Repeat("█", other)},
		{s.dim, strings.Repeat("░", free)},
	}
}

func (m Model) cleanupPanel() [][]piece {
	s := m.styles
	if m.dk.view.AutoCleanEvery <= 0 {
		return [][]piece{
			{{s.bold, " Auto-cleanup off"}},
			{{s.sub, " d and b remove worktrees by hand"}},
		}
	}
	return [][]piece{
		{{s.bold, " Auto-cleanup every " + everyText(m.dk.view.AutoCleanEvery)}},
		{{s.sub, " merged + clean + no session → removed"}},
		{{s.sub, " merged + dirty → backup, then asks"}},
	}
}

func (m Model) depsPanel() [][]piece {
	s := m.styles
	d := m.dk.view.DepsStore
	if d == nil {
		return nil
	}
	return [][]piece{
		{{s.bold, " Shared deps store"}},
		{{s.sub, " " + filepath.Base(d.Path) + " " + sizeText(d.Size) + " once, cloned per worktree"}},
	}
}

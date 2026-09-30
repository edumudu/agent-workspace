package tui

import (
	"fmt"
	"path"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// seg is a run of text in one foreground; empty fields fall back to the
// row's defaults.
type seg struct {
	text string
	fg   string
	bold bool
}

// painter caches one lipgloss style per color pair, so a frame of
// highlighted code builds no styles.
type painter struct {
	theme  Theme
	styles map[[2]string]lipgloss.Style
	bolds  map[[2]string]lipgloss.Style
}

func newPainter(t Theme) *painter {
	return &painter{theme: t, styles: map[[2]string]lipgloss.Style{}, bolds: map[[2]string]lipgloss.Style{}}
}

func (p *painter) style(fg, bg string, bold bool) lipgloss.Style {
	if fg == "" {
		fg = p.theme.Text
	}
	cache := p.styles
	if bold {
		cache = p.bolds
	}
	k := [2]string{fg, bg}
	if st, ok := cache[k]; ok {
		return st
	}
	st := lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Bold(bold)
	if bg != "" {
		st = st.Background(lipgloss.Color(bg))
	}
	cache[k] = st
	return st
}

// cell renders segs in exactly width columns on bg, cutting what does not
// fit.
func (p *painter) cell(width int, bg string, segs ...seg) string {
	var b strings.Builder
	used := 0
	for _, s := range segs {
		if used >= width {
			break
		}
		text := s.text
		if w := ansi.StringWidth(text); used+w > width {
			text = ansi.Truncate(text, width-used, "")
		}
		used += ansi.StringWidth(text)
		b.WriteString(p.style(s.fg, bg, s.bold).Render(text))
	}
	if used < width {
		b.WriteString(p.style("", bg, false).Render(strings.Repeat(" ", width-used)))
	}
	return b.String()
}

// cellRight puts right against the cell's right edge, cutting left first.
func (p *painter) cellRight(width int, bg string, left, right []seg) string {
	rw := 0
	for _, s := range right {
		rw += ansi.StringWidth(s.text)
	}
	if rw >= width {
		return p.cell(width, bg, right...)
	}
	return p.cell(width-rw, bg, left...) + p.cell(rw, bg, right...)
}

const (
	railWidth = 5
	treeMax   = 34
	// reviewChrome is every row that is not the file tree or diff: top bar,
	// scope bar, worktree bar, rule and footer.
	reviewChrome = 5
)

func (m Model) reviewWidths() (area, tree, diff int) {
	area = max(m.width-railWidth-1, 20)
	tree = min(treeMax, area/3)
	return area, tree, area - tree - 1
}

// diffHeight is how many diff rows fit under the file header.
func (m Model) diffHeight() int { return max(m.height-reviewChrome-1, 1) }

func (m Model) reviewView() string {
	t := m.opts.Theme
	p := m.paint
	area, treeW, diffW := m.reviewWidths()
	bodyH := max(m.height-reviewChrome, 1)

	var lines []string
	lines = append(lines, m.topBar())
	right := []string{m.scopeBar(area), m.worktreeBar(area), p.cell(area, "", seg{text: strings.Repeat("─", area), fg: t.Surface})}
	tree := m.treeLines(treeW, bodyH)
	diff := m.diffLines(diffW, bodyH)
	sep := p.style(t.Surface, "", false).Render("│")
	for i := range bodyH {
		right = append(right, tree[i]+sep+diff[i])
	}
	right = append(right, m.reviewFooter(area))
	rail := m.railLines(len(right))
	for i, r := range right {
		lines = append(lines, rail[i]+sep+r)
	}
	return strings.Join(lines, "\n")
}

func (m Model) railLines(n int) []string {
	t := m.opts.Theme
	p := m.paint
	out := make([]string, 0, n)
	for _, e := range m.entries {
		bar, bg := " ", ""
		if e.session.ID == m.rv.session {
			bar, bg = "▌", t.Selected
		}
		g := m.glyph(e.session)
		fg := t.Overlay
		switch g.s {
		case "✳", "●":
			fg = t.Peach
		case "○":
		default:
			fg = t.Blue
		}
		out = append(out,
			p.cell(railWidth, bg, seg{text: bar, fg: t.Blue}, seg{text: " " + g.s, fg: fg}),
			p.cell(railWidth, bg, seg{text: bar, fg: t.Blue}, seg{text: fmt.Sprintf(" %d", e.num), fg: t.Subtext}),
			p.cell(railWidth, ""))
	}
	for len(out) < n {
		out = append(out, p.cell(railWidth, ""))
	}
	return out[:n]
}

func scopeLabel(s domain.ReviewScope) string {
	switch s {
	case domain.ScopeLastTurn:
		return "last turn"
	case domain.ScopeBranch:
		return "branch vs base"
	}
	return "uncommitted"
}

func (m Model) chip(label string, on bool) seg {
	if on {
		return seg{text: " " + label + " ", fg: m.opts.Theme.Base, bold: true}
	}
	return seg{text: " " + label + " ", fg: m.opts.Theme.Text}
}

// chips renders a row of chips, the active one on bg, the rest on surface.
func (m Model) chips(lead seg, labels []string, active int, onBg string, rightSegs []seg, width int) string {
	t := m.opts.Theme
	p := m.paint
	var b strings.Builder
	used := ansi.StringWidth(lead.text)
	b.WriteString(p.style(lead.fg, "", lead.bold).Render(lead.text))
	rw := 0
	for _, s := range rightSegs {
		rw += ansi.StringWidth(s.text)
	}
	for i, l := range labels {
		c := m.chip(l, i == active)
		w := ansi.StringWidth(c.text) + 1
		if used+w > width-rw {
			break
		}
		bg := t.Mantle
		if i == active {
			bg = onBg
		}
		b.WriteString(p.style(c.fg, bg, c.bold).Render(c.text))
		b.WriteString(" ")
		used += w
	}
	return b.String() + p.cellRight(width-used, "", nil, rightSegs)
}

func (m Model) scopeBar(width int) string {
	t := m.opts.Theme
	files, add, del, viewed := 0, 0, 0, 0
	for _, f := range m.rv.prep.files {
		files++
		add += f.file.Added
		del += f.file.Deleted
		if m.rv.viewed(f) {
			viewed++
		}
	}
	active := 0
	labels := make([]string, len(domain.ReviewScopes))
	for i, s := range domain.ReviewScopes {
		labels[i] = scopeLabel(s)
		if s == m.rv.scope {
			active = i
		}
	}
	right := []seg{
		{text: count(files, "file") + "  ", fg: t.Subtext},
		{text: fmt.Sprintf("+%d", add), fg: t.Green},
		{text: "  "},
		{text: fmt.Sprintf("-%d", del), fg: t.Red},
		{text: fmt.Sprintf("  %d/%d viewed ", viewed, files), fg: t.Subtext},
	}
	if m.rv.loading {
		right = append([]seg{{text: "loading…  ", fg: t.Overlay}}, right...)
	}
	return m.chips(seg{text: " REVIEW  ", fg: t.Subtext, bold: true}, labels, active, t.Blue, right, width)
}

func (m Model) worktreeBar(width int) string {
	t := m.opts.Theme
	labels := []string{"all"}
	active := 0
	for _, e := range m.entries {
		if e.session.ID != m.rv.session {
			continue
		}
		for _, w := range e.worktrees {
			if w.ID == m.rv.worktree {
				active = len(labels)
			}
			labels = append(labels, worktreeChip(w))
		}
	}
	return m.chips(seg{text: " worktree  ", fg: t.Subtext}, labels, active, t.Text, nil, width)
}

func worktreeChip(w domain.Worktree) string {
	if w.PR != nil {
		return fmt.Sprintf("%s #%d", worktreeLabel(w), w.PR.Number)
	}
	return worktreeLabel(w)
}

func statusColor(t Theme, s domain.FileStatus) string {
	switch s {
	case domain.FileAdded:
		return t.Green
	case domain.FileDeleted:
		return t.Red
	case domain.FileRenamed:
		return t.Blue
	}
	return t.Peach
}

func (m Model) treeLines(width, height int) []string {
	t := m.opts.Theme
	p := m.paint
	var rows []string
	curRow := 0
	for _, r := range m.rv.prep.tree {
		if r.file < 0 {
			if len(rows) > 0 {
				rows = append(rows, p.cell(width, ""))
			}
			var pr []seg
			if r.wt.PR != nil {
				pr = []seg{{text: fmt.Sprintf("#%d ", r.wt.PR.Number), fg: t.Subtext}}
			}
			rows = append(rows, p.cellRight(width, "", []seg{{text: " " + worktreeLabel(r.wt), bold: true}}, pr))
			if r.err != "" {
				rows = append(rows, p.cell(width, "", seg{text: "   ! " + r.err, fg: t.Overlay}))
			}
			continue
		}
		f := m.rv.prep.files[r.file]
		bg := ""
		if r.file == m.rv.cur {
			bg, curRow = t.Selected, len(rows)
		}
		mark := seg{text: "   "}
		if m.rv.viewed(f) {
			mark = seg{text: " ✓ ", fg: t.Green}
		}
		stat := fmt.Sprintf("+%d -%d ", f.file.Added, f.file.Deleted)
		if f.file.Deleted == 0 {
			stat = fmt.Sprintf("+%d ", f.file.Added)
		}
		rows = append(rows, p.cellRight(width, bg,
			[]seg{mark, {text: string(f.file.Status) + " ", fg: statusColor(t, f.file.Status), bold: true}, {text: path.Base(f.file.Path)}},
			[]seg{{text: stat, fg: t.Subtext}}))
	}
	if len(m.rv.prep.files) == 0 && !m.rv.loading {
		msg := "  no changes"
		if m.rv.err != "" {
			msg = "  " + m.rv.err
		}
		rows = append(rows, p.cell(width, "", seg{text: msg, fg: t.Overlay}))
	}
	off := 0
	if curRow >= height {
		off = curRow - height + 1
	}
	rows = rows[min(off, len(rows)):]
	for len(rows) < height {
		rows = append(rows, p.cell(width, ""))
	}
	return rows[:height]
}

const numWidth = 5

func (m Model) diffLines(width, height int) []string {
	t := m.opts.Theme
	p := m.paint
	out := make([]string, 0, height)
	f, ok := m.rv.current()
	if !ok {
		for len(out) < height {
			out = append(out, p.cell(width, ""))
		}
		return out
	}
	unified, split := m.chip("unified", !m.rv.split), m.chip("split", m.rv.split)
	ubg, sbg := t.Surface, t.Base
	if m.rv.split {
		ubg, sbg = t.Base, t.Surface
	}
	unified.fg, split.fg = t.Text, t.Text
	head := []seg{{text: " "}, {text: " " + worktreeLabel(f.wt) + " ", fg: t.Subtext}, {text: "  " + f.file.Path + "  ", bold: true},
		{text: fmt.Sprintf("+%d", f.file.Added), fg: t.Green}, {text: "  "}, {text: fmt.Sprintf("-%d", f.file.Deleted), fg: t.Red}}
	toggle := p.cell(ansi.StringWidth(unified.text), ubg, unified) + " " + p.cell(ansi.StringWidth(split.text), sbg, split) + " "
	out = append(out, p.cell(width-ansi.StringWidth(ansi.Strip(toggle)), "", head...)+toggle)

	rows := f.unified
	if m.rv.split {
		rows = f.split
	}
	switch {
	case f.file.Binary:
		out = append(out, p.cell(width, "", seg{text: "  binary file", fg: t.Overlay}))
	case len(rows) == 0:
		out = append(out, p.cell(width, "", seg{text: "  no text changes", fg: t.Overlay}))
	}
	for i := m.rv.scroll; i < len(rows) && len(out) < height; i++ {
		out = append(out, m.diffRow(rows[i], width))
	}
	for len(out) < height {
		out = append(out, p.cell(width, ""))
	}
	return out
}

func (m Model) diffRow(r diffRow, width int) string {
	t := m.opts.Theme
	p := m.paint
	if r.header != "" {
		return p.cell(width, t.Mantle, seg{text: " " + cleanText(r.header), fg: t.Subtext})
	}
	if r.one != nil {
		return m.codeCell(r.one, width, sideBoth)
	}
	half := (width - 1) / 2
	return m.codeCell(r.left, half, sideOld) + p.style(t.Surface, "", false).Render("│") + m.codeCell(r.right, width-1-half, sideNew)
}

type side int

const (
	sideBoth side = iota
	sideOld
	sideNew
)

// codeCell draws a line with its numbers: old and new in the unified view,
// the side's own in the split view. A nil line is a blank side.
func (m Model) codeCell(sl *styledLine, width int, sd side) string {
	t := m.opts.Theme
	p := m.paint
	if sl == nil {
		return p.cell(width, t.Mantle)
	}
	num := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", numWidth)
		}
		return fmt.Sprintf("%*d", numWidth, n)
	}
	bg, sign, signFg := "", " ", t.Text
	switch sl.line.Kind {
	case domain.LineAdded:
		bg, sign, signFg = t.AddedBg, "+", t.Green
	case domain.LineDeleted:
		bg, sign, signFg = t.DeletedBg, "-", t.Red
	}
	var nums string
	switch sd {
	case sideBoth:
		nums = num(sl.line.Old) + " " + num(sl.line.New)
	case sideOld:
		nums = num(sl.line.Old)
	default:
		nums = num(sl.line.New)
	}
	segs := make([]seg, 0, len(sl.toks)+2)
	segs = append(segs, seg{text: nums, fg: t.Overlay}, seg{text: " " + sign + "  ", fg: signFg})
	for _, tk := range sl.toks {
		segs = append(segs, seg{text: tk.text, fg: tk.color})
	}
	return p.cell(width, bg, segs...)
}

func (m Model) reviewFooter(width int) string {
	t := m.opts.Theme
	var segs []seg
	for _, k := range [][2]string{{"v", "viewed"}, {"u", "split"}, {"[ ]", "scope"}, {"w", "worktree"}, {"n/p", "file"}, {"j/k", "scroll"}, {"r", "close"}} {
		segs = append(segs, seg{text: " " + k[0], bold: true}, seg{text: " " + k[1] + "  ", fg: t.Subtext})
	}
	var right []seg
	if m.status != "" {
		right = []seg{{text: m.status + " ", fg: t.Peach}}
	}
	return m.paint.cellRight(width, "", segs, right)
}

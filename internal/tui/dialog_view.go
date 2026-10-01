package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// dialogMaxWidth keeps the popup's form readable on a wide terminal.
const dialogMaxWidth = 96

// dialogColumnsFrom is the narrowest form that puts harness, model and
// effort side by side; below it they stack.
const dialogColumnsFrom = 72

// dialogScreen is the whole popup: the form centred in the terminal.
func (m Model) dialogScreen() string {
	f := m
	f.width = min(m.width-4, dialogMaxWidth)
	lines, keep := f.dialogLines()
	lines = dialogViewport(lines, keep, m.height)
	margin := strings.Repeat(" ", max((m.width-f.width)/2, 0))
	for i, l := range lines {
		lines[i] = margin + l
	}
	if len(lines) < m.height {
		return "\n" + strings.Join(lines, "\n")
	}
	return strings.Join(lines, "\n")
}

// dialogViewport fits the form into height rows: the title bar and the
// action row stay, and the rows between scroll so keep stays in view.
func dialogViewport(lines []string, keep, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	if height < 3 {
		return lines[len(lines)-height:]
	}
	title, body, actions := lines[0], lines[1:len(lines)-1], lines[len(lines)-1]
	room := height - 2
	start := min(max(keep-1-room/2, 0), len(body)-room)
	return append(append([]string{title}, body[start:start+room]...), actions)
}

// dialogLines draws the form at m.width, as in docs/images/new-session.png.
// It also returns the row to keep in view: the active field, or the status
// line once there is one.
func (m Model) dialogLines() ([]string, int) {
	s := m.styles
	d := m.dialog
	w := m.width
	var out []string
	keep := 0
	add := func(f field, lines ...string) {
		if d.field == f {
			keep = len(out)
		}
		out = append(out, lines...)
	}
	out = append(out, m.titleBar(" New session", "esc "), "")

	item := []piece{{s.text, d.workItem}}
	if d.field == fieldWorkItem {
		item = append(item, piece{s.bar, "▏"})
	} else if d.workItem == "" {
		item = []piece{{s.dim, "a Linear issue, a GitHub PR or a few words"}}
	}
	add(fieldWorkItem, append([]string{m.label(fieldWorkItem, "Work item")}, m.box(w, item, d.field == fieldWorkItem)...)...)
	out = append(out, m.line(false, []piece{{s.sub, " " + workItemPreview(d.workItem)}}, nil), "")

	ws := []piece{{s.dim, "none: run agentws workspace add <path>"}}
	facts := ""
	if len(d.spaces) > 0 {
		space := d.spaces[d.ws]
		ws = []piece{{s.bold, "‹ " + filepath.Base(space.Root) + " ›"}, {s.sub, "  " + workspaceKind(space)}}
		facts = space.Root
		if space.Root == d.last {
			facts += " · last used"
		}
	}
	add(fieldWorkspace, append([]string{m.label(fieldWorkspace, "Workspace")}, m.box(w, ws, d.field == fieldWorkspace)...)...)
	out = append(out, m.line(false, []piece{{s.sub, " " + facts}}, nil), "")

	out = append(out, m.pickers(&keep, len(out))...)
	out = append(out, "")
	out = append(out, m.startsAt()...)
	if lines := m.adviceBox(); len(lines) > 0 {
		out = append(append(out, ""), lines...)
	}
	out = append(out, "")
	switch {
	case d.busy && !d.started:
		keep = len(out)
		out = append(out, m.line(false, []piece{{s.sub, " starting…"}}, nil))
	case d.err != "":
		keep = len(out)
		out = append(out, m.line(false, []piece{{s.peach, " ✗ " + d.err}}, nil))
	}
	hint := []piece{{s.dim, " ⇥ next · ←/→ change"}}
	buttons := []piece{{s.sub, "esc cancel"}, {s.text, "  "}, {s.badge, " ⏎ create "}, {s.text, " "}}
	return append(out, m.line(false, hint, buttons)), keep
}

// titleBar fills the whole width with the badge colour, as the mockup's
// blue header does.
func (m Model) titleBar(left, right string) string {
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return m.styles.badge.Render(left + strings.Repeat(" ", gap) + right)
}

func (m Model) label(f field, name string) string {
	st := m.styles.bold
	if m.dialog.field == f {
		st = m.styles.brand
	}
	return m.line(false, []piece{{st, " " + name}}, nil)
}

// box draws content in a rounded frame, blue while its field is active.
func (m Model) box(w int, content []piece, active bool) []string {
	border := m.styles.dim
	if active {
		border = m.styles.bar
	}
	inner := max(w-4, 1)
	var used int
	var fitted []piece
	for _, p := range content {
		pw := ansi.StringWidth(p.s)
		if used+pw > inner-1 {
			fitted = append(fitted, piece{p.st, ansi.Truncate(p.s, max(inner-1-used, 0), "…")})
			used = inner - 1
			break
		}
		fitted = append(fitted, p)
		used += pw
	}
	mid := border.Render(" │ ")
	for _, p := range fitted {
		mid += p.st.Render(p.s)
	}
	mid += strings.Repeat(" ", max(inner-1-used, 0)) + border.Render("│")
	return []string{
		border.Render(" ╭" + strings.Repeat("─", inner) + "╮"),
		mid,
		border.Render(" ╰" + strings.Repeat("─", inner) + "╯"),
	}
}

// pickers draws harness, model and effort in three columns, or stacked when
// the form is narrow. keep moves to the active picker's row.
func (m Model) pickers(keep *int, at int) []string {
	s := m.styles
	d := m.dialog
	harness := make([]piece, 0, 2*len(harnessChoices))
	for i, h := range harnessChoices {
		if i == d.harness {
			harness = append(harness, piece{s.brand, "● " + h})
		} else {
			harness = append(harness, piece{s.dim, "○ " + h})
		}
		harness = append(harness, piece{s.text, "  "})
	}
	model := d.model
	if model == "" {
		model = "default"
	}
	effort := d.efforts[d.effort]
	if effort == "" {
		effort = "default"
	}
	cols := []struct {
		f     field
		name  string
		value []piece
	}{
		{fieldHarness, "Harness", harness},
		{fieldModel, "Model", []piece{{s.bold, "‹ " + model + " ›"}}},
		{fieldEffort, "Effort", []piece{{s.bold, "‹ " + effort + " ›"}}},
	}
	if m.width < dialogColumnsFrom {
		var out []string
		for _, c := range cols {
			if d.field == c.f {
				*keep = at + len(out)
			}
			out = append(out, m.label(c.f, c.name), m.line(false, append([]piece{{s.text, " "}}, c.value...), nil))
		}
		return out
	}
	colWidth := (m.width - 1) / len(cols)
	var labels, values []piece
	for _, c := range cols {
		st := s.bold
		if d.field == c.f {
			st, *keep = s.brand, at
		}
		labels = append(labels, padTo(colWidth, piece{st, " " + c.name})...)
		values = append(values, padTo(colWidth, append([]piece{{s.text, " "}}, c.value...)...)...)
	}
	return []string{m.line(false, labels, nil), m.line(false, values, nil)}
}

func padTo(width int, ps ...piece) []piece {
	used := 0
	for _, p := range ps {
		used += ansi.StringWidth(p.s)
	}
	return append(ps, piece{lipgloss.NewStyle(), strings.Repeat(" ", max(width-used, 0))})
}

func workspaceKind(w domain.Workspace) string {
	if w.Kind == "" {
		return "new · added when you create"
	}
	if w.Kind == domain.WorkspaceSingle {
		return "single repo"
	}
	return fmt.Sprintf("orchestration root · %d repos", len(w.Repos))
}

// workItemPreview says what the work item was read as and what the session
// will be called.
func workItemPreview(input string) string {
	if strings.TrimSpace(input) == "" {
		return "the session is named after it"
	}
	t := domain.ParseWorkItem(input)
	switch t.Source {
	case domain.TaskLinear:
		title := t.IssueTitle
		if title == "" {
			title = "title from Linear"
		}
		return t.Ref + " · " + title + " · session will be named after it"
	case domain.TaskPR:
		return t.Ref + " · session will be named after the PR"
	}
	return "text · session named “" + t.Text + "”"
}

// startsAt explains where the session will run, from the same plan the
// daemon makes.
func (m Model) startsAt() []string {
	s := m.styles
	d := m.dialog
	if len(d.spaces) == 0 {
		return nil
	}
	space := d.spaces[d.ws]
	if space.Kind == "" {
		bar := piece{s.dim, " ▌ "}
		return []string{
			m.line(false, []piece{bar, {s.bold, "Starts in " + filepath.Base(space.Root)}}, nil),
			m.line(false, []piece{bar, {s.sub, "agentws checks whether it is one repo or a folder of repos when you create"}}, nil),
		}
	}
	plan := domain.PlanSessionStart(space, domain.TaskSlug(domain.ParseWorkItem(d.workItem)), "", nil)
	title := "Starts at the " + filepath.Base(space.Root) + " root"
	detail := "The agent creates worktrees as it needs them; each one attaches to this session."
	if wt := plan.Worktree; wt != nil {
		title = "Starts in a fresh worktree"
		name := wt.Repo
		if strings.TrimSpace(d.workItem) != "" {
			name += ":" + wt.Branch
		}
		detail = name + " from " + wt.Base
	}
	bar := piece{s.dim, " ▌ "}
	return []string{
		m.line(false, []piece{bar, {s.bold, title}}, nil),
		m.line(false, []piece{bar, {s.sub, detail}}, nil),
	}
}

// adviceBox is the low-quota warning, with the switch to the other harness
// when there is one to offer.
func (m Model) adviceBox() []string {
	advice, ok := m.advice()
	if !ok {
		return nil
	}
	s := m.styles
	low := advice.Low
	head := fmt.Sprintf("%s %s window %d%% used", harnessName(low.Harness), domain.WindowLabel(low.Window), usedPercent(low))
	if reset := resetClock(low.ResetsAt, m.opts.Now()); reset != "" {
		head += " · resets " + reset
	}
	bar := piece{s.peach, " ▌ "}
	var action []piece
	detail := "No " + harnessName(advice.Other) + " figures yet"
	if offer, ok := m.fallbackOffer(); ok {
		action = []piece{{s.need, "ctrl+s start in " + string(offer.Request.Harness) + " "}}
		detail = fmt.Sprintf("%s %s is %d%% used", harnessName(offer.Request.Harness), domain.WindowLabel(offer.Advice.OtherShortest.Window), usedPercent(*offer.Advice.OtherShortest))
		if offer.Request.Model != "" {
			detail += " · starts as " + offer.Request.Model
		}
	} else if o := advice.OtherShortest; o != nil {
		detail = fmt.Sprintf("%s %s is %d%% used", harnessName(advice.Other), domain.WindowLabel(o.Window), usedPercent(*o))
		if advice.Other != domain.HarnessCodex {
			action = []piece{{s.need, "ctrl+s start in " + string(advice.Other) + " "}}
		}
	}
	if m.width < dialogColumnsFrom {
		out := []string{
			m.line(false, []piece{bar, {s.need, "! " + head}}, nil),
			m.line(false, []piece{bar, {s.sub, "  " + detail}}, nil),
		}
		if len(action) > 0 {
			out = append(out, m.line(false, append([]piece{bar, {s.text, "  "}}, action...), nil))
		}
		return out
	}
	return []string{
		m.line(false, []piece{bar, {s.need, "! " + head}}, action),
		m.line(false, []piece{bar, {s.sub, "  " + detail}}, nil),
	}
}

func harnessName(h domain.Harness) string {
	if h == domain.HarnessCodex {
		return "Codex"
	}
	return "Claude"
}

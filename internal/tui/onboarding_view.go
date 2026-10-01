package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

const setupMaxWidth = 80

var stepNames = map[domain.OnboardStep]string{
	domain.OnboardPick:   "Agents",
	domain.OnboardClaude: "Claude Code",
	domain.OnboardCodex:  "Codex",
	domain.OnboardNvim:   "Neovim",
	domain.OnboardFinish: "Done",
}

// why: it is laid out like the new-session popup (ADR 0037) so both read as one app.
func (m Model) setupScreen() string {
	f := m
	f.width = min(m.width-4, setupMaxWidth)
	lines := dialogViewport(f.setupLines(), 0, m.height)
	margin := strings.Repeat(" ", max((m.width-f.width)/2, 0))
	for i, l := range lines {
		lines[i] = margin + l
	}
	if len(lines) < m.height {
		return "\n" + strings.Join(lines, "\n")
	}
	return strings.Join(lines, "\n")
}

func (m Model) setupLines() []string {
	s := m.styles
	ob := m.ob
	out := []string{m.titleBar(" Set up agentws", "esc skip "), "", m.stepTrail(), ""}
	switch ob.step {
	case domain.OnboardPick:
		out = append(out, m.pickLines()...)
	case domain.OnboardClaude, domain.OnboardCodex:
		out = append(out, m.harnessLines()...)
	case domain.OnboardNvim:
		out = append(out, m.nvimLines()...)
	case domain.OnboardFinish:
		out = append(out, m.finishLines()...)
	}
	out = append(out, "")
	switch {
	case ob.busy:
		out = append(out, m.line(false, []piece{{s.sub, " working…"}}, nil))
	case ob.err != "":
		out = append(out, m.line(false, []piece{{s.peach, " ✗ " + ob.err}}, nil))
	}
	hint, action := m.setupActions()
	buttons := []piece{{s.sub, "esc skip"}, {s.text, "  "}, {s.badge, " ⏎ " + action + " "}, {s.text, " "}}
	return append(out, m.line(false, []piece{{s.dim, hint}}, buttons))
}

func (m Model) setupActions() (hint, action string) {
	ob := m.ob
	switch ob.step {
	case domain.OnboardPick:
		return " ↑/↓ move · space pick", "next"
	case domain.OnboardClaude, domain.OnboardCodex:
		_, setup := ob.harness()
		if domain.HarnessOffer(setup) == domain.OfferInstall && ob.results[ob.step] != resultInstalled {
			return " s skip", "install"
		}
		return "", "next"
	case domain.OnboardNvim:
		return " s skip", "next"
	}
	return "", "done"
}

func (m Model) stepTrail() string {
	s := m.styles
	ob := m.ob
	var ps []piece
	passed := true
	for i, step := range ob.steps() {
		if i > 0 {
			ps = append(ps, piece{s.dim, " › "})
		}
		st := s.dim
		switch {
		case step == ob.step:
			st, passed = s.brand, false
		case passed:
			st = s.sub
		}
		ps = append(ps, piece{st, stepNames[step]})
	}
	return m.line(false, append([]piece{{s.text, " "}}, ps...), nil)
}

func (m Model) pickLines() []string {
	s := m.styles
	ob := m.ob
	rows := [][]piece{}
	for i, h := range []struct {
		name   string
		picked bool
		setup  domain.HarnessSetup
	}{{"Claude Code", ob.claude, ob.status.Claude}, {"Codex", ob.codex, ob.status.Codex}} {
		cursor, check, name := "  ", "[ ] ", s.text
		if i == ob.cursor {
			cursor, name = "▸ ", s.brand
		}
		if h.picked {
			check = "[x] "
		}
		rows = append(rows, []piece{{s.bar, cursor}, {name, check + padRight(h.name, 14)}, setupState(s, h.setup)})
	}
	out := []string{m.line(false, []piece{{s.bold, " Which agents do you use?"}}, nil)}
	out = append(out, m.framed(rows, true)...)
	return append(out, m.para(piece{s.text, " "}, s.sub, "agentws reads their hooks to show what each session is doing. Pick one or both; you set up each one next.")...)
}

func setupState(s styles, h domain.HarnessSetup) piece {
	switch domain.HarnessOffer(h) {
	case domain.OfferInstalled:
		return piece{s.green, "✓ set up"}
	case domain.OfferBroken:
		return piece{s.peach, "✗ cannot read its config"}
	}
	return piece{s.dim, "not set up"}
}

func (m Model) harnessLines() []string {
	s := m.styles
	ob := m.ob
	h, setup := ob.harness()
	name, change, undo := "Claude Code", "Adds the agentws hooks and wraps your status line. Your own hooks and status line keep running.", "agentws setup claude --remove"
	if h == domain.HarnessCodex {
		name, change, undo = "Codex", "Adds the agentws hooks. Your other hooks stay as they are.", "agentws setup codex --remove"
	}
	bar := piece{s.dim, " ▌ "}
	out := []string{m.line(false, []piece{{s.bold, " " + name}}, []piece{setupState(s, setup), {s.text, " "}}), ""}
	if domain.HarnessOffer(setup) == domain.OfferBroken {
		out = append(out, m.para(piece{s.peach, " ▌ "}, s.text, setup.Err)...)
		return append(out, m.para(piece{s.peach, " ▌ "}, s.sub, "Fix or move the file, then open this again with S.")...)
	}
	out = append(out, m.line(false, []piece{bar, {s.sub, "Changes "}, {s.bold, setup.File}}, nil))
	out = append(out, m.para(bar, s.text, change)...)
	backup := setup.Backup
	if backup == "" {
		backup = "none needed: the file does not exist yet"
	}
	out = append(out, m.line(false, []piece{bar, {s.sub, "Backup  "}, {s.text, backup}}, nil))
	out = append(out, m.line(false, []piece{bar, {s.sub, "Undo    "}, {s.text, undo}}, nil))
	if h == domain.HarnessCodex {
		trust := piece{s.peach, " ▌ "}
		out = append(out, "", m.line(false, []piece{trust, {s.need, "! One more step in Codex"}}, nil))
		out = append(out, m.para(trust, s.sub, domain.CodexTrustStep)...)
	}
	return out
}

func (m Model) nvimLines() []string {
	s := m.styles
	n := m.ob.status.Nvim
	bar := piece{s.dim, " ▌ "}
	out := []string{m.line(false, []piece{{s.bold, " Neovim"}}, nil), ""}
	switch domain.NvimOfferFor(n) {
	case domain.NvimMissing:
		return append(out, m.para(bar, s.text, "nvim is not on PATH. Install Neovim 0.10+ to open files and diffs and write review comments from nvim; open this again with S afterwards.")...)
	case domain.NvimReady:
		out = append(out, m.line(false, []piece{bar, {s.green, "✓ the agentws plugin is configured"}}, nil))
		return append(out, m.para(bar, s.sub, "found in "+n.ConfigFile)...)
	case domain.NvimNoPlugin:
		out = append(out, m.line(false, []piece{{s.peach, " ▌ "}, {s.text, "The plugin files are not at " + n.PluginDir + "."}}, nil))
		out = append(out, m.para(piece{s.peach, " ▌ "}, s.sub, "The install script puts them there; from a source checkout, point the snippet at its nvim/ directory.")...)
		out = append(out, "")
	}
	out = append(out, m.line(false, []piece{{s.text, " Add this to "}, {s.bold, n.ConfigFile}}, nil))
	out = append(out, "")
	for _, l := range domain.NvimSnippet(n.PluginDir, n.ConfigFile) {
		// why: no frame and no truncation, so selecting the lines copies exactly the code.
		out = append(out, "   "+s.teal.Render(l))
	}
	out = append(out, "")
	return append(out, m.para(piece{s.text, " "}, s.sub, "agentws never edits your nvim config: copy the lines in, then restart nvim.")...)
}

func (m Model) finishLines() []string {
	s := m.styles
	ob := m.ob
	out := []string{m.line(false, []piece{{s.bold, " You're set"}}, nil), ""}
	row := func(step domain.OnboardStep, name string, picked bool, setup domain.HarnessSetup) {
		mark, detail := piece{s.dim, "  – "}, "not picked"
		switch {
		case picked && ob.results[step] == resultSkipped:
			detail = "skipped"
		case picked && setup.Installed:
			mark, detail = piece{s.green, "  ✓ "}, "hooks installed"
		case picked:
			detail = "not set up"
		}
		out = append(out, m.line(false, []piece{mark, {s.text, padRight(name, 14)}, {s.sub, detail}}, nil))
	}
	row(domain.OnboardClaude, "Claude Code", ob.claude, ob.status.Claude)
	row(domain.OnboardCodex, "Codex", ob.codex, ob.status.Codex)
	mark, detail := piece{s.dim, "  – "}, ""
	switch {
	case ob.results[domain.OnboardNvim] == resultSkipped:
		detail = "skipped"
	case domain.NvimOfferFor(ob.status.Nvim) == domain.NvimReady:
		mark, detail = piece{s.green, "  ✓ "}, "plugin configured"
	case domain.NvimOfferFor(ob.status.Nvim) == domain.NvimMissing:
		detail = "not on PATH"
	default:
		detail = "add the snippet to " + ob.status.Nvim.ConfigFile
	}
	out = append(out, m.line(false, []piece{mark, {s.text, padRight("Neovim", 14)}, {s.sub, detail}}, nil), "")
	return append(out, m.para(piece{s.text, " "}, s.sub, "Open this again with S in the sidebar or agentws setup. n starts your first session.")...)
}

// why: a long path or message wraps under its bar instead of being cut off.
func (m Model) para(bar piece, st lipgloss.Style, text string) []string {
	room := max(m.width-ansi.StringWidth(bar.s)-1, 10)
	var out []string
	for _, l := range strings.Split(ansi.Wordwrap(text, room, ""), "\n") {
		out = append(out, m.line(false, []piece{bar, {st, l}}, nil))
	}
	return out
}

func (m Model) framed(rows [][]piece, active bool) []string {
	border := m.styles.dim
	if active {
		border = m.styles.bar
	}
	inner := max(m.width-4, 1)
	out := []string{border.Render(" ╭" + strings.Repeat("─", inner) + "╮")}
	for _, r := range rows {
		used := 0
		mid := border.Render(" │ ")
		for _, p := range r {
			pw := ansi.StringWidth(p.s)
			if used+pw > inner-1 {
				mid += p.st.Render(ansi.Truncate(p.s, max(inner-1-used, 0), "…"))
				used = inner - 1
				break
			}
			mid += p.st.Render(p.s)
			used += pw
		}
		out = append(out, mid+strings.Repeat(" ", max(inner-1-used, 0))+border.Render("│"))
	}
	return append(out, border.Render(" ╰"+strings.Repeat("─", inner)+"╯"))
}

func padRight(s string, n int) string {
	return s + strings.Repeat(" ", max(n-ansi.StringWidth(s), 0))
}

package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

var spinner = []string{"◐", "◓", "◑", "◒"}

type styles struct {
	text, sub, dim, bold, brand, header, need lipgloss.Style
	blue, peach, teal, green                  lipgloss.Style
	bar, badge                                lipgloss.Style
	selectedBg, base                          lipgloss.Style
}

func newStyles(t Theme) styles {
	fg := func(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }
	return styles{
		text:       fg(t.Text),
		sub:        fg(t.Subtext),
		dim:        fg(t.Overlay),
		bold:       fg(t.Text).Bold(true),
		brand:      fg(t.Blue).Bold(true),
		header:     fg(t.Subtext).Bold(true),
		need:       fg(t.Peach).Bold(true),
		blue:       fg(t.Blue),
		peach:      fg(t.Peach),
		teal:       fg(t.Teal),
		green:      fg(t.Green),
		bar:        fg(t.Blue),
		badge:      lipgloss.NewStyle().Foreground(lipgloss.Color(t.Base)).Background(lipgloss.Color(t.Blue)).Bold(true),
		selectedBg: lipgloss.NewStyle().Background(lipgloss.Color(t.Selected)),
		base:       lipgloss.NewStyle().Background(lipgloss.Color(t.Base)),
	}
}

type piece struct {
	st lipgloss.Style
	s  string
}

// line lays left and right pieces out across width, truncating the left
// side first. With sel set, every cell gets the selection background.
func (m Model) line(sel bool, left, right []piece) string {
	w := m.width
	render := func(ps []piece) (string, int) {
		var b strings.Builder
		n := 0
		for _, p := range ps {
			st := p.st
			if sel {
				st = st.Background(lipgloss.Color(m.opts.Theme.Selected))
			}
			b.WriteString(st.Render(p.s))
			n += ansi.StringWidth(p.s)
		}
		return b.String(), n
	}
	r, rw := render(right)
	room := w - rw
	if rw > 0 {
		room--
	}
	var plain []piece
	used := 0
	for _, p := range left {
		pw := ansi.StringWidth(p.s)
		if used+pw > room {
			if cut := room - used; cut > 0 {
				plain = append(plain, piece{p.st, ansi.Truncate(p.s, cut, "…")})
			}
			break
		}
		plain = append(plain, p)
		used += pw
	}
	l, lw := render(plain)
	gap := w - lw - rw
	if gap < 0 {
		gap = 0
	}
	pad := strings.Repeat(" ", gap)
	if sel {
		pad = m.styles.selectedBg.Render(pad)
	}
	return l + pad + r
}

func (m Model) View() tea.View {
	if m.rv.open {
		v := tea.NewView(m.reviewView())
		v.AltScreen = true
		return v
	}
	if m.dk.open {
		v := tea.NewView(m.diskScreen())
		v.AltScreen = true
		return v
	}
	s := m.styles
	var lines []string
	lines = append(lines, m.topBar())
	lines = append(lines, m.limitLines()...)
	lines = append(lines, "")

	need := 0
	for _, e := range m.entries {
		if e.session.NeedsYou() {
			need++
		}
	}
	var right []piece
	if need > 0 {
		right = []piece{{s.need, fmt.Sprintf("%d need you", need)}}
	}
	lines = append(lines, m.line(false, []piece{{s.header, " SESSIONS"}}, right))

	body, selRow := m.body()
	footer := append(m.cardLines(), m.footer()...)
	room := m.height - len(lines) - len(footer)
	if room < 0 {
		room = 0
	}
	off := 0
	if selRow >= room {
		off = selRow - room + 3
	}
	if off > len(body)-room {
		off = max(0, len(body)-room)
	}
	body = body[off:]
	if len(body) > room {
		body = body[:room]
	}
	lines = append(lines, body...)
	for len(lines)+len(footer) < m.height {
		lines = append(lines, "")
	}
	lines = append(lines, footer...)

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func (m Model) topBar() string {
	s := m.styles
	left := []piece{{s.bar, "▌"}, {s.brand, "agentws"}}
	for _, slot := range []struct{ name, val string }{{"claude", m.top.Claude}, {"codex", m.top.Codex}} {
		if slot.val != "" {
			left = append(left, piece{s.bold, "  " + slot.name + " "}, piece{s.text, slot.val})
		}
	}
	var right []piece
	if m.top.Disk != "" {
		right = append(right, piece{s.sub, "disk " + m.top.Disk + "  "})
	}
	right = append(right, piece{s.bold, m.opts.Now().Format("15:04")})
	return m.line(false, left, right)
}

// body renders the sidebar rows and returns the row the selection starts on.
func (m Model) body() ([]string, int) {
	s := m.styles
	if m.dialog != nil {
		return m.dialogLines()
	}
	if m.help {
		return m.helpLines(), 0
	}
	if m.picker != nil {
		return m.pickerLines(), 0
	}
	if len(m.entries) == 0 {
		return []string{
			"",
			m.line(false, []piece{{s.sub, " No sessions yet."}}, nil),
			m.line(false, []piece{{s.dim, " Sessions you start show up here."}}, nil),
		}, 0
	}
	var out []string
	selRow := 0
	for _, e := range m.entries {
		if e.groupStart {
			out = append(out, "", m.line(false, []piece{{s.sub, " " + taskLabel(e.task)}}, []piece{{s.sub, strings.Join(e.groupRepos, " ")}}))
		}
		sel := e.session.ID == m.selected
		if sel {
			selRow = len(out)
		}
		out = append(out, m.sessionLines(e, sel)...)
	}
	return out, selRow
}

func (m Model) sessionLines(e entry, sel bool) []string {
	s := m.styles
	bar := piece{s.text, " "}
	if sel {
		bar = piece{s.bar, "▌"}
	}
	x := e.session
	glyph := m.glyph(x)
	tag := piece{s.blue.Bold(true), "CC"}
	if x.Harness == domain.HarnessCodex {
		tag = piece{s.teal.Bold(true), "CX"}
	}
	name := domain.NameFor(e.task, entryPRs(e))
	if name == "" {
		name = x.ID
	}
	out := []string{m.line(sel,
		[]piece{bar, glyph, {s.text, fmt.Sprintf(" %d ", e.num)}, {s.bold, name}},
		append(m.muteMarker(x), tag, piece{s.text, " "}))}

	detail := strings.TrimSpace(x.Model + " " + x.Effort)
	trees := "root only"
	if n := len(x.WorktreeIDs); n > 0 {
		trees = count(n, "worktree")
	}
	var open []piece
	if label := portLabel(e.ports()); label != "" {
		open = []piece{{s.teal, label}, {s.text, " "}}
	}
	out = append(out, m.line(sel, []piece{bar, {s.sub, fmt.Sprintf("    %s  ctx %d%%  %s", detail, x.Usage.ContextLeftPercent, trees)}}, append(open, m.switchMarks(x)...)))
	if m.collapsed[x.ID] {
		return out
	}
	for _, w := range e.worktrees {
		pr := piece{s.dim, "–"}
		if w.PR != nil {
			pr = piece{s.sub, fmt.Sprintf("#%d", w.PR.Number)}
		}
		right := []piece{pr, {s.text, "   "}}
		if label := portLabel(w.Ports); label != "" {
			right = append([]piece{{s.teal, label + " "}}, right...)
		}
		out = append(out, m.line(sel,
			[]piece{bar, {s.dim, "     └ "}, {s.text, worktreeLabel(w)}},
			right))
	}
	return append(out, m.subagentLines(x.ID, sel)...)
}

func (m Model) muteMarker(x domain.Session) []piece {
	if !x.Muted {
		return nil
	}
	return []piece{{m.styles.dim, "⊘ "}}
}

func (m Model) glyph(x domain.Session) piece {
	s := m.styles
	switch {
	case x.State == domain.StateRunning:
		return piece{s.blue, spinner[m.frame%len(spinner)]}
	case x.NeedsYou():
		return piece{s.peach, "✳"}
	case x.State == domain.StateDone && x.Unread:
		return piece{s.peach, "●"}
	}
	return piece{s.dim, "○"}
}

func (m Model) helpLines() []string {
	s := m.styles
	keys := [][2]string{
		{"1-9", "jump to session"},
		{"space", "next waiting"},
		{"tab", "last session"},
		{"j / k", "move"},
		{"o", "expand / collapse worktrees"},
		{"m", "mute session"},
		{"R", "rename and pin the name"},
		{"A", "unpin (name is automatic)"},
		{"K", "kill the session's dev servers"},
		{"w", "worktrees and disk"},
		{"M / E", "switch model / effort"},
		{"enter", "focus agent pane"},
		{`ctrl+\`, "in an agent pane: back to the sidebar"},
		{"n", "new session"},
		{"x", "end session"},
		{"?", "close help"},
		{"q", "quit (sessions keep running)"},
	}
	out := []string{"", m.line(false, []piece{{s.header, " KEYS"}}, nil)}
	for _, k := range keys {
		out = append(out, m.line(false, []piece{{s.bold, fmt.Sprintf(" %-7s", k[0])}, {s.sub, k[1]}}, nil))
	}
	return out
}

func (m Model) footer() []string {
	s := m.styles
	worktrees := 0
	for _, e := range m.entries {
		worktrees += len(e.session.WorktreeIDs)
	}
	right := []piece{{s.sub, count(len(m.entries), "session") + " · " + count(worktrees, "worktree") + " "}}
	left, withCounts := m.statusLeft()
	switch {
	case m.status != "":
		right = []piece{{s.peach, m.status + " "}}
	case !withCounts:
		right = nil
	}
	return []string{
		"",
		m.line(false, []piece{{s.bold, " ⏎"}, {s.sub, " focus      "}, {s.bold, "␣"}, {s.sub, " next waiting"}}, nil),
		m.line(false, []piece{{s.bold, " ⇥"}, {s.sub, " last       "}, {s.bold, "?"}, {s.sub, " keys"}}, nil),
		m.line(false, left, right),
	}
}

// statusLeft is the status line's left side. Ports and the kill prompt need
// the room the counts would take, so they leave the counts out.
func (m Model) statusLeft() (left []piece, withCounts bool) {
	s := m.styles
	if m.renaming != nil {
		return m.renameLeft(), false
	}
	if m.confirm != nil {
		return []piece{{s.badge, " KILL "}, {s.peach, " kill " + m.confirm.label + "? y/n"}}, false
	}
	if i := m.index(m.selected); i >= 0 {
		if label := portLabel(m.entries[i].ports()); label != "" {
			return []piece{{s.badge, " SESSION "}, {s.teal, " " + label}, {s.sub, " · K kill"}}, false
		}
	}
	return []piece{{s.badge, " SESSION "}, {s.sub, " j/k move · ? keys"}}, true
}

func taskLabel(t domain.Task) string {
	title := t.PinnedName
	for _, c := range []string{t.IssueTitle, t.Text} {
		if title == "" {
			title = c
		}
	}
	switch {
	case t.Ref != "" && title != "":
		return t.Ref + " · " + title
	case t.Ref != "":
		return t.Ref
	case title != "":
		return title
	}
	return t.ID
}

func worktreeLabel(w domain.Worktree) string {
	repo := w.Repo
	if repo != "" {
		// why: Repo is the main checkout's path; its last element is the name people use.
		repo = filepath.Base(repo)
	}
	part := w.SubtaskSlug
	if part == "" {
		part = w.Branch
	}
	if part == "" {
		return repo
	}
	return repo + ":" + part
}

func count(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// minCardHeight is the shortest pane that still has room for the card.
const minCardHeight = 24

func (m Model) cardLines() []string {
	i := m.index(m.selected)
	if i < 0 || m.help || m.height < minCardHeight {
		return nil
	}
	s := m.styles
	e := m.entries[i]
	card := domain.BuildSessionCard(e.task, e.session, e.worktrees, m.events[e.session.ID])
	out := []string{"", m.line(false, []piece{{s.header, " CARD"}}, nil)}

	var title []string
	for _, part := range []string{card.Ref, card.Title} {
		if part != "" {
			title = append(title, part)
		}
	}
	label := strings.Join(title, " · ")
	if label == "" {
		label = e.session.ID
	}
	out = append(out, m.line(false, []piece{{s.bold, " " + label}}, nil))
	out = append(out, m.nameLines(cleanText(card.Name))...)

	if len(card.PRs) > 0 {
		chips := []piece{{s.dim, " PRs "}}
		for _, pr := range card.PRs {
			chips = append(chips, piece{s.blue, fmt.Sprintf("#%d ", pr.Number)})
		}
		out = append(out, m.line(false, chips, nil))
		out = append(out, m.prBoardLines(card.PRs)...)
	}
	for j, action := range card.Actions {
		lead := "       "
		if j == 0 {
			lead = " last  "
		}
		out = append(out, m.line(false, []piece{{s.dim, lead}, {s.sub, cleanText(action)}}, nil))
	}
	if card.Waiting != "" {
		reason := "waiting on you"
		if e.session.State == domain.StatePermission {
			reason = "asks permission"
		}
		out = append(out, m.line(false, []piece{{s.need, " ✳ " + reason}}, nil))
		for _, line := range strings.Split(card.Waiting, "\n") {
			out = append(out, m.line(false, []piece{{s.text, "   " + cleanText(line)}}, nil))
		}
	}
	return out
}

// prBoardLines is the board under the card's PR chips. A PR the daemon has
// no state for yet gets none.
func (m Model) prBoardLines(prs []domain.PullRequest) []string {
	s := m.styles
	var out []string
	for _, pr := range prs {
		if pr.State == "" {
			continue
		}
		head := fmt.Sprintf(" #%d ", pr.Number)
		switch {
		case pr.State != domain.PROpen:
			out = append(out, m.line(false, []piece{{s.blue, head}, {s.dim, strings.ToLower(string(pr.State))}}, nil))
			continue
		case pr.ReadyToMerge():
			out = append(out, m.line(false, []piece{{s.blue, head}, {s.green, "ready to merge"}}, nil))
		default:
			out = append(out, m.line(false, []piece{{s.blue, head}, {s.need, "blocked"}}, nil))
			for _, b := range pr.Blockers() {
				out = append(out, m.line(false, []piece{{s.sub, "   " + b}}, nil))
			}
		}
		for _, f := range pr.Failing {
			out = append(out, m.line(false, []piece{{s.peach, "   ✗ " + link(f.URL, cleanText(f.Name))}}, nil))
		}
		if pr.BotComments > 0 {
			out = append(out, m.line(false, []piece{{s.dim, "   " + count(pr.BotComments, "bot comment") + " since push"}}, nil))
		}
	}
	return out
}

// nameLines is the session's full name, wrapped, since the sidebar row cuts it.
func (m Model) nameLines(name string) []string {
	if name == "" {
		return nil
	}
	const lead = " name  "
	var out []string
	for i, part := range strings.Split(ansi.Wrap(name, max(m.width-len(lead), 1), ""), "\n") {
		prefix := strings.Repeat(" ", len(lead))
		if i == 0 {
			prefix = lead
		}
		out = append(out, m.line(false, []piece{{m.styles.dim, prefix}, {m.styles.text, part}}, nil))
	}
	return out
}

// link wraps text in an OSC 8 hyperlink when url is a plain http(s) URL.
// The URL comes from GitHub, so anything with a control character or another
// scheme is left as text rather than risk breaking out of the sequence.
func link(url, text string) string {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return text
	}
	for _, r := range url {
		if r < ' ' || r == 0x7f {
			return text
		}
	}
	return ansi.SetHyperlink(url) + text + ansi.ResetHyperlink()
}

// cleanText drops escape sequences and control characters an agent put in
// its text, so they cannot repaint the terminal.
func cleanText(s string) string {
	s = strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

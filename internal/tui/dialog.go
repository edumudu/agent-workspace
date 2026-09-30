package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// startTimeout covers the setup recipe, which may install dependencies.
const startTimeout = 15 * time.Minute

const callTimeout = 5 * time.Second

type field int

const (
	fieldWorkItem field = iota
	fieldWorkspace
	fieldHarness
	fieldModel
	fieldEffort
	fieldCount
)

var (
	harnessChoices = []string{string(domain.HarnessClaude), string(domain.HarnessCodex)}
	effortChoices  = []string{"", "low", "medium", "high"}
)

// ws, harness and effort index roots, harnessChoices and effortChoices.
type dialog struct {
	field    field
	workItem string
	model    string
	roots    []string
	kinds    []domain.WorkspaceKind
	ws       int
	harness  int
	effort   int
	err      string
	busy     bool
}

type sessionStartedMsg struct{ session domain.Session }

type startFailedMsg struct{ err error }

func (m Model) openDialog() Model {
	d := &dialog{}
	all := sorted(m.workspaces)
	last, found := domain.LastUsedWorkspace(all)
	for i, w := range all {
		d.roots = append(d.roots, w.Root)
		d.kinds = append(d.kinds, w.Kind)
		if found && w.Root == last.Root {
			d.ws = i
		}
	}
	m.dialog = d
	return m
}

func sorted(ws map[string]domain.Workspace) []domain.Workspace {
	out := make([]domain.Workspace, 0, len(ws))
	for _, w := range ws {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Root < out[j].Root })
	return out
}

func cycle(i, delta, n int) int {
	if n == 0 {
		return 0
	}
	return ((i+delta)%n + n) % n
}

func (d *dialog) text() *string {
	switch d.field {
	case fieldWorkItem:
		return &d.workItem
	case fieldModel:
		return &d.model
	}
	return nil
}

func (d *dialog) change(delta int) {
	switch d.field {
	case fieldWorkspace:
		d.ws = cycle(d.ws, delta, len(d.roots))
	case fieldHarness:
		d.harness = cycle(d.harness, delta, len(harnessChoices))
	case fieldEffort:
		d.effort = cycle(d.effort, delta, len(effortChoices))
	}
}

// own gives m a copy of the dialog, so an earlier Model value never
// changes with it.
func (m *Model) own() *dialog {
	d := *m.dialog
	m.dialog = &d
	return &d
}

func (m Model) dialogPaste(s string) Model {
	m.own()
	if t := m.dialog.text(); t != nil && !m.dialog.busy {
		*t += strings.ReplaceAll(s, "\n", " ")
	}
	return m
}

func (m Model) dialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := m.own()
	if d.busy {
		return m, nil
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		m.dialog = nil
	case "tab", "down":
		d.field = field(cycle(int(d.field), 1, int(fieldCount)))
	case "shift+tab", "up":
		d.field = field(cycle(int(d.field), -1, int(fieldCount)))
	case "left":
		d.change(-1)
	case "right":
		d.change(1)
	case "ctrl+s":
		if advice, ok := m.advice(); ok && advice.OtherShortest != nil {
			d.harness = cycle(d.harness, 1, len(harnessChoices))
		}
	case "enter":
		return m, m.submit()
	case "backspace":
		if t := d.text(); t != nil && *t != "" {
			r := []rune(*t)
			*t = string(r[:len(r)-1])
		}
	default:
		if msg.Text != "" {
			return m.dialogPaste(msg.Text), nil
		}
	}
	return m, nil
}

func (m Model) submit() tea.Cmd {
	d := m.dialog
	switch {
	case strings.TrimSpace(d.workItem) == "":
		d.err = "work item is empty"
		return nil
	case len(d.roots) == 0:
		d.err = "no workspace: run agentws workspace add <path>"
		return nil
	}
	c := m.opts.Calls
	if c == nil {
		d.err = "not connected to the daemon"
		return nil
	}
	d.err, d.busy = "", true
	p := rpc.NewSessionParams{
		Workspace: d.roots[d.ws],
		WorkItem:  strings.TrimSpace(d.workItem),
		Harness:   harnessChoices[d.harness],
		Model:     strings.TrimSpace(d.model),
		Effort:    effortChoices[d.effort],
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
		defer cancel()
		var s domain.Session
		if err := c.Call(ctx, rpc.MethodNewSession, p, &s); err != nil {
			return startFailedMsg{err}
		}
		return sessionStartedMsg{s}
	}
}

func (m Model) call(method string, params any) tea.Cmd {
	c := m.opts.Calls
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		if err := c.Call(ctx, method, params, nil); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

// showNew puts a just-started session in view, which swaps its pane into
// the main slot.
func (m Model) showNew(id string) tea.Cmd {
	return m.call(rpc.MethodSessionFocus, rpc.SessionFocusParams{ID: id})
}

func (m Model) endSelected() tea.Cmd {
	if m.selected == "" {
		return nil
	}
	return m.call(rpc.MethodEndSession, rpc.SessionRef{ID: m.selected})
}

func (m Model) advice() (domain.SwitchAdvice, bool) {
	sessions := make([]domain.Session, 0, len(m.sessions))
	for _, x := range m.sessions {
		sessions = append(sessions, x)
	}
	return domain.Advise(domain.Quotas(sessions), domain.Harness(harnessChoices[m.dialog.harness]))
}

func (m Model) adviceLine() (string, bool) {
	advice, ok := m.advice()
	if !ok {
		return "", false
	}
	s := m.styles
	low := advice.Low
	left := []piece{{s.peach, fmt.Sprintf(" ⚠ %s %s %d%% left", low.Harness, domain.WindowLabel(low.Window), low.LeftPercent)}}
	if o := advice.OtherShortest; o != nil {
		left = append(left, piece{s.dim, " · "}, piece{s.bold, "ctrl+s"}, piece{s.sub, fmt.Sprintf(" %s %d%%", advice.Other, o.LeftPercent)})
	}
	return m.line(false, left, nil), true
}

func workItemKind(input string) string {
	t := domain.ParseWorkItem(input)
	switch t.Source {
	case domain.TaskLinear:
		return "linear " + t.Ref
	case domain.TaskPR:
		return "PR " + t.Ref
	}
	return "text"
}

func (m Model) dialogLines() []string {
	s := m.styles
	d := m.dialog
	row := func(f field, label string, value []piece) string {
		bar := piece{s.text, " "}
		if d.field == f {
			bar = piece{s.bar, "▌"}
		}
		return m.line(false, append([]piece{bar, {s.sub, fmt.Sprintf("%-10s ", label)}}, value...), nil)
	}
	choice := func(v string) []piece { return []piece{{s.bold, "‹ " + v + " ›"}} }
	input := func(v string, f field) []piece {
		cursor := ""
		if d.field == f {
			cursor = "▏"
		}
		return []piece{{s.text, v + cursor}}
	}

	ws := []piece{{s.dim, "none"}}
	if len(d.roots) > 0 {
		ws = append(choice(filepath.Base(d.roots[d.ws])), piece{s.dim, "  " + string(d.kinds[d.ws])})
	}
	effort := effortChoices[d.effort]
	if effort == "" {
		effort = "default"
	}
	model := input(d.model, fieldModel)
	if d.model == "" && d.field != fieldModel {
		model = []piece{{s.dim, "default"}}
	}
	out := []string{
		"",
		m.line(false, []piece{{s.header, " NEW SESSION"}}, nil),
		"",
		row(fieldWorkItem, "Work item", input(d.workItem, fieldWorkItem)),
		m.line(false, []piece{{s.dim, fmt.Sprintf(" %-11s%s", "", workItemKind(d.workItem))}}, nil),
		row(fieldWorkspace, "Workspace", ws),
		row(fieldHarness, "Harness", choice(harnessChoices[d.harness])),
	}
	if line, ok := m.adviceLine(); ok {
		out = append(out, line)
	}
	out = append(out,
		row(fieldModel, "Model", model),
		row(fieldEffort, "Effort", choice(effort)),
		"",
	)
	switch {
	case d.busy:
		out = append(out, m.line(false, []piece{{s.sub, " starting…"}}, nil))
	case d.err != "":
		out = append(out, m.line(false, []piece{{s.peach, " ✗ " + d.err}}, nil))
	}
	return append(out, m.line(false, []piece{{s.dim, " ⏎ start · ⇥ next · ←/→ change · esc cancel"}}, nil))
}

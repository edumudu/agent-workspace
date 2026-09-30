package tui

import (
	"cmp"
	"context"
	"slices"
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
	spaces   []domain.Workspace
	// last is the last used workspace root, named on its facts line.
	last    string
	ws      int
	harness int
	effort  int
	// efforts is effortChoices plus any mapped effort a fallback brought that the picker lacks.
	efforts []string
	// fallback remembers the Claude start a fallback replaced, so going back restores it.
	fallback *fallbackTrace
	err      string
	busy     bool
	// seq tells this dialog's start reply from one sent by a dialog closed earlier.
	seq      int
	defaults map[domain.Harness]Defaults
}

type fallbackTrace struct {
	fromModel, model string
	fromEffort       int
	effort           int
}

type sessionStartedMsg struct {
	seq     int
	session domain.Session
}

type startFailedMsg struct {
	seq int
	err error
}

func (m Model) openDialog() Model {
	m.dialogs++
	d := &dialog{seq: m.dialogs, defaults: m.opts.Defaults, efforts: slices.Clone(effortChoices)}
	start := m.opts.Defaults[domain.HarnessClaude]
	d.model, d.effort = start.Model, effortIndex(d.efforts, start.Effort)
	all := sorted(m.workspaces)
	last, found := domain.LastUsedWorkspace(all)
	d.spaces = all
	for i, w := range all {
		if found && w.Root == last.Root {
			d.ws, d.last = i, w.Root
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

func effortIndex(choices []string, effort string) int {
	return max(slices.Index(choices, effort), 0)
}

// cycleHarness moves to the next harness and carries the model and effort over
// to its defaults, unless the user changed them from the previous defaults.
func (d *dialog) cycleHarness(delta int) {
	prev := d.defaults[domain.Harness(harnessChoices[d.harness])]
	d.harness = cycle(d.harness, delta, len(harnessChoices))
	next := d.defaults[domain.Harness(harnessChoices[d.harness])]
	restoredModel, restoredEffort := false, false
	if f := d.fallback; f != nil {
		d.fallback = nil
		if d.model == f.model {
			d.model, restoredModel = f.fromModel, true
		}
		if d.effort == f.effort {
			d.effort, restoredEffort = f.fromEffort, true
		}
	}
	if !restoredModel && d.model == prev.Model {
		d.model = next.Model
	}
	if !restoredEffort && d.efforts[d.effort] == prev.Effort {
		d.effort = effortIndex(d.efforts, next.Effort)
	}
}

// takeFallback moves the dialog to the Codex start the offer proposes; a field
// the mapping leaves empty gets Codex's own default. An effort the picker does
// not list is added to it so it survives to submission.
func (d *dialog) takeFallback(req domain.StartRequest) {
	trace := &fallbackTrace{fromModel: d.model, fromEffort: d.effort}
	d.harness = slices.Index(harnessChoices, string(req.Harness))
	defaults := d.defaults[req.Harness]
	d.model = cmp.Or(req.Model, defaults.Model)
	effort := cmp.Or(req.Effort, defaults.Effort)
	if !slices.Contains(d.efforts, effort) {
		d.efforts = append(slices.Clone(d.efforts), effort)
	}
	d.effort = effortIndex(d.efforts, effort)
	trace.model, trace.effort = d.model, d.effort
	d.fallback = trace
}

func cycle(i, delta, n int) int {
	if n == 0 {
		return 0
	}
	return ((i+delta)%n + n) % n
}

func (d *dialog) text() *string {
	if d.field == fieldWorkItem {
		return &d.workItem
	}
	return nil
}

// models is the default, the harness's switch choices, then the current model
// when a default or a fallback brought one the list lacks.
func (d *dialog) models() []string {
	out := append([]string{""}, domain.SwitchChoices(domain.Harness(harnessChoices[d.harness]), domain.SwitchModel)...)
	if !slices.Contains(out, d.model) {
		out = append(out, d.model)
	}
	return out
}

func (d *dialog) change(delta int) {
	switch d.field {
	case fieldWorkspace:
		d.ws = cycle(d.ws, delta, len(d.spaces))
	case fieldHarness:
		d.cycleHarness(delta)
	case fieldModel:
		models := d.models()
		d.model = models[cycle(slices.Index(models, d.model), delta, len(models))]
	case fieldEffort:
		d.effort = cycle(d.effort, delta, len(d.efforts))
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
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.opts.NewSessionOnly {
			return m, tea.Quit
		}
		// why: a start already sent keeps going; its reply still selects and shows the session.
		m.dialog = nil
		return m, nil
	}
	if d.busy {
		return m, nil
	}
	switch msg.String() {
	case "tab", "down":
		d.field = field(cycle(int(d.field), 1, int(fieldCount)))
	case "shift+tab", "up":
		d.field = field(cycle(int(d.field), -1, int(fieldCount)))
	case "left":
		d.change(-1)
	case "right":
		d.change(1)
	case "ctrl+s":
		if offer, ok := m.fallbackOffer(); ok {
			d.takeFallback(offer.Request)
		} else if advice, ok := m.advice(); ok && advice.OtherShortest != nil && advice.Other != domain.HarnessCodex {
			d.cycleHarness(1)
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
	case len(d.spaces) == 0:
		d.err = "no workspace: run agentws workspace add <path>"
		return nil
	}
	c := m.opts.Calls
	if c == nil {
		d.err = "not connected to the daemon"
		return nil
	}
	d.err, d.busy = "", true
	seq := d.seq
	p := rpc.NewSessionParams{
		Workspace: d.spaces[d.ws].Root,
		WorkItem:  strings.TrimSpace(d.workItem),
		Harness:   harnessChoices[d.harness],
		Model:     strings.TrimSpace(d.model),
		Effort:    d.efforts[d.effort],
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
		defer cancel()
		var s domain.Session
		if err := c.Call(ctx, rpc.MethodNewSession, p, &s); err != nil {
			return startFailedMsg{seq, err}
		}
		return sessionStartedMsg{seq, s}
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

// showNewAndQuit ends the popup's program only once the new session is in
// view, so closing the popup never races the focus call.
func (m Model) showNewAndQuit(id string) tea.Cmd {
	show := m.showNew(id)
	return func() tea.Msg {
		if show != nil {
			show()
		}
		return tea.QuitMsg{}
	}
}

type popupFailedMsg struct{}

// openPopup asks the daemon to run the dialog in a popup; if it cannot, the
// dialog opens inline instead.
func (m Model) openPopup() tea.Cmd {
	c, p := m.opts.Calls, m.opts.DialogPopup
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		if err := c.Call(ctx, rpc.MethodClientPopup, p, nil); err != nil {
			return popupFailedMsg{}
		}
		return nil
	}
}

func (m Model) endSession(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	return m.call(rpc.MethodEndSession, rpc.SessionRef{ID: id})
}

func (m Model) quotas() []domain.Quota {
	sessions := make([]domain.Session, 0, len(m.sessions))
	for _, x := range m.sessions {
		sessions = append(sessions, x)
	}
	return domain.Current(domain.Quotas(sessions), m.opts.Now())
}

func (m Model) chosenHarness() domain.Harness {
	return domain.Harness(harnessChoices[m.dialog.harness])
}

func (m Model) advice() (domain.SwitchAdvice, bool) {
	return domain.AdviseAt(m.quotas(), m.chosenHarness(), m.warnThreshold())
}

func (m Model) warnThreshold() int {
	if t := m.opts.Fallback.Threshold; t > 0 {
		return t
	}
	return domain.WarnQuotaLeft
}

func (m Model) fallbackOffer() (domain.FallbackOffer, bool) {
	d := m.dialog
	return domain.OfferFallback(m.quotas(), m.opts.Fallback, domain.StartRequest{
		Harness: m.chosenHarness(),
		Model:   strings.TrimSpace(d.model),
		Effort:  d.efforts[d.effort],
	})
}

package tui

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

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

type dialog struct {
	field      field
	workItem   string
	model      string
	spaces     []domain.Workspace
	last       string
	ws         int
	harness    int
	effort     int
	efforts    []string
	fallback   *fallbackTrace
	err        string
	busy       bool
	started    bool
	seq        int
	defaults   map[domain.Harness]Defaults
	path       string
	base, home string
	listing    listing
	listings   int
	pick       int
}

type listing struct {
	req    int
	dir    string
	dirs   []domain.Child
	done   bool
	failed bool
}

type dirsListedMsg struct {
	seq    int
	req    int
	dir    string
	dirs   []domain.Child
	failed bool
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
	if dir := m.opts.LaunchDir; dir != "" {
		d.startIn(dir)
	}
	d.home = m.opts.Home
	d.base = cmp.Or(m.opts.LaunchDir, d.root(), d.home, "/")
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
	switch d.field {
	case fieldWorkItem:
		return &d.workItem
	case fieldWorkspace:
		return &d.path
	}
	return nil
}

func (d *dialog) root() string {
	if len(d.spaces) == 0 {
		return ""
	}
	return d.spaces[d.ws].Root
}

func (d *dialog) typed() domain.PathInput {
	return domain.ParsePathInput(d.path, d.base, d.home)
}

func (d *dialog) chosen() (domain.Workspace, bool) {
	if d.path == "" {
		if len(d.spaces) == 0 {
			return domain.Workspace{}, false
		}
		return d.spaces[d.ws], true
	}
	root := d.typed().Path
	for _, w := range d.spaces {
		if w.Root == root && w.Kind != "" {
			return w, true
		}
	}
	return domain.Workspace{Root: root}, true
}

func (d *dialog) matches() []domain.Child {
	in := d.typed()
	if d.path == "" || !d.listing.done || d.listing.dir != in.Dir {
		return nil
	}
	return domain.CompleteDirs(d.listing.dirs, in.Prefix)
}

type folderState int

const (
	folderPending folderState = iota
	folderFound
	folderMissing
)

func (d *dialog) folder() folderState {
	in := d.typed()
	if !d.listing.done || d.listing.dir != in.Dir {
		return folderPending
	}
	if d.listing.failed {
		return folderMissing
	}
	if in.Prefix == "" || in.Prefix == "." || in.Prefix == ".." {
		return folderFound
	}
	for _, c := range d.listing.dirs {
		if c.Name == in.Prefix {
			return folderFound
		}
	}
	return folderMissing
}

func (d *dialog) open() bool {
	ms := d.matches()
	if len(ms) == 0 {
		return false
	}
	head := d.path[:strings.LastIndex(d.path, "/")+1]
	if d.path == "~" {
		head = "~/"
	}
	d.path = head + ms[min(d.pick, len(ms)-1)].Name + "/"
	return true
}

func (d *dialog) up() {
	t := strings.TrimSuffix(d.path, "/")
	cut := strings.LastIndex(t, "/")
	seg := t[cut+1:]
	switch {
	case t == "":
		d.path = "/"
	case t == ".":
		d.path = "../"
	case seg == "..":
		d.path = t + "/../"
	case t == "~":
		d.path = filepath.Dir(d.home) + "/"
	case cut < 0:
		d.path = "./"
	default:
		d.path = t[:cut+1]
	}
}

func (m Model) listDirs() tea.Cmd {
	d := m.dialog
	d.pick = 0
	c := m.opts.Calls
	if d.path == "" || c == nil {
		return nil
	}
	dir := d.typed().Dir
	if d.listing.dir == dir && !d.listing.failed {
		return nil
	}
	d.listings++
	d.listing = listing{req: d.listings, dir: dir}
	seq, req := d.seq, d.listings
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		var out rpc.WorkspaceDirs
		err := c.Call(ctx, rpc.MethodWorkspaceDirs, rpc.WorkspaceDirsParams{Path: dir}, &out)
		return dirsListedMsg{seq: seq, req: req, dir: dir, dirs: out.Dirs, failed: err != nil}
	}
}

func (m Model) gotDirs(msg dirsListedMsg) Model {
	if m.dialog == nil || m.dialog.seq != msg.seq || m.dialog.listing.req != msg.req {
		return m
	}
	m.own().listing = listing{req: msg.req, dir: msg.dir, dirs: msg.dirs, done: true, failed: msg.failed}
	return m
}

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

func (m *Model) own() *dialog {
	d := *m.dialog
	m.dialog = &d
	return &d
}

func (m Model) dialogPaste(s string) (Model, tea.Cmd) {
	m.own()
	if t := m.dialog.text(); t != nil && !m.dialog.busy {
		*t += strings.ReplaceAll(s, "\n", " ")
	}
	if m.dialog.field == fieldWorkspace {
		return m, m.listDirs()
	}
	return m, nil
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
		m.dialog = nil
		return m, nil
	}
	if d.busy || d.started {
		return m, nil
	}
	if d.field == fieldWorkspace && d.path != "" {
		if next, cmd, ok := m.pathKey(msg.String()); ok {
			return next, cmd
		}
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
			return m, m.listDirs()
		}
	default:
		if msg.Text != "" {
			return m.dialogPaste(msg.Text)
		}
	}
	return m, nil
}

func (m Model) pathKey(key string) (tea.Model, tea.Cmd, bool) {
	d := m.dialog
	n := len(d.matches())
	switch {
	case key == "down" && n > 0:
		d.pick = min(d.pick+1, n-1)
	case key == "up" && n > 0:
		d.pick = max(d.pick-1, 0)
	case key == "right":
		if d.open() {
			return m, m.listDirs(), true
		}
	case key == "left":
		d.up()
		return m, m.listDirs(), true
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) submit() tea.Cmd {
	d := m.dialog
	switch {
	case strings.TrimSpace(d.workItem) == "":
		d.err = "work item is empty"
		return nil
	}
	space, ok := d.chosen()
	if !ok {
		d.err = "no workspace: type a folder's path"
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
		Workspace: space.Root,
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

func (m Model) showNew(id string) tea.Cmd {
	return m.call(rpc.MethodSessionFocus, rpc.SessionFocusParams{ID: id})
}

func (m Model) showNewAndQuit(id string) tea.Cmd {
	show := m.showNew(id)
	return func() tea.Msg {
		if show != nil {
			if e, ok := show().(errMsg); ok {
				return showFailedMsg(e)
			}
		}
		return tea.QuitMsg{}
	}
}

type showFailedMsg struct{ err error }

type popupFailedMsg struct{}

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

func (d *dialog) startIn(dir string) {
	best := -1
	for i, w := range d.spaces {
		if (dir == w.Root || strings.HasPrefix(dir, w.Root+"/")) && (best < 0 || len(w.Root) > len(d.spaces[best].Root)) {
			best = i
		}
	}
	if best >= 0 {
		d.ws = best
		return
	}
	d.spaces = append([]domain.Workspace{{Root: dir}}, d.spaces...)
	d.ws = 0
}

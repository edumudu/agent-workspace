// Package tui is the Bubble Tea client. It renders only from the daemon's
// state snapshot and diffs, and never runs commands or touches disk while
// rendering.
package tui

import (
	"context"
	"fmt"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// Focuser moves keyboard focus to the agent pane; *rpc.Client is one.
type Focuser interface {
	FocusMain(ctx context.Context) error
}

// Attender tells the daemon what the user did about a session's attention
// state; *rpc.Client is one.
type Attender interface {
	MuteSession(ctx context.Context, id string, muted bool) error
	FocusSession(ctx context.Context, id string) error
}

// Killer ends the process groups behind ports; *rpc.Client is one.
type Killer interface {
	KillPorts(ctx context.Context, pgids []int) ([]int, error)
}

type Options struct {
	Theme Theme
	// Now is the clock for the top bar; nil means time.Now.
	Now func() time.Time
	// Tick is the spinner and clock interval; zero turns the ticker off.
	Tick  time.Duration
	Focus Focuser
	// Attend may be nil, which turns off m and the seen marker on enter.
	Attend Attender
	// Kill may be nil, which turns off K.
	Kill Killer
	// Defaults pre-fill the new-session dialog per harness.
	Defaults map[domain.Harness]Defaults
	// Fallback sets when the dialog warns of low Claude quota and what it offers in Codex.
	Fallback domain.FallbackConfig
	// Calls may be nil, which turns off n and x.
	Calls Caller
	// Switch applies model and effort switches; nil turns M and E off.
	Switch Switcher
	// Review may be nil, which turns off r.
	Review Reviewer
	// Disk may be nil, which turns off w.
	Disk Disker
}

// Caller makes daemon calls such as session.new; *rpc.Client is one.
type Caller interface {
	Call(ctx context.Context, method string, params, out any) error
}

// StateMsg replaces the whole state, as a subscribe snapshot does.
type StateMsg rpc.State

type DiffMsg rpc.Diff

// TickMsg advances the one ticker every running glyph shares.
type TickMsg struct{}

// TopBarMsg fills the top bar slots; an empty field hides its slot.
type TopBarMsg struct {
	Claude string
	Codex  string
	Disk   string
}

type DisconnectedMsg struct{ Err error }

type errMsg struct{ err error }

type entry struct {
	session    domain.Session
	task       domain.Task
	num        int
	groupStart bool
	groupRepos []string
	worktrees  []domain.Worktree
}

type Model struct {
	opts   Options
	styles styles
	width  int
	height int

	workspaces map[string]domain.Workspace
	tasks      map[string]domain.Task
	taskOrder  []string
	worktrees  map[string]domain.Worktree
	sessions   map[string]domain.Session
	events     map[string][]domain.SessionEvent
	subagents  map[string][]domain.Subagent
	entries    []entry
	collapsed  map[string]bool

	selected string
	last     string
	help     bool
	picker   *picker
	frame    int
	top      TopBarMsg
	status   string
	confirm  *killPrompt
	rv       reviewState
	dk       diskState
	paint    *painter
	renaming *renamePrompt

	dialog  *dialog
	dialogs int
	// ending is the session an open end confirmation is about.
	ending string
	// pending is a session just started, selected once its diff arrives.
	pending string
}

func New(opts Options) Model {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return Model{
		opts:       opts,
		styles:     newStyles(opts.Theme),
		width:      40,
		height:     24,
		workspaces: map[string]domain.Workspace{},
		tasks:      map[string]domain.Task{},
		worktrees:  map[string]domain.Worktree{},
		sessions:   map[string]domain.Session{},
		events:     map[string][]domain.SessionEvent{},
		subagents:  map[string][]domain.Subagent{},
		collapsed:  map[string]bool{},
		paint:      newPainter(opts.Theme),
	}
}

// Selected is the ID of the highlighted session, or "" when there is none.
func (m Model) Selected() string { return m.selected }

func (m Model) Init() tea.Cmd { return m.tick() }

func (m Model) tick() tea.Cmd {
	if m.opts.Tick <= 0 {
		return nil
	}
	return tea.Tick(m.opts.Tick, func(time.Time) tea.Msg { return TickMsg{} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case StateMsg:
		m.load(rpc.State(msg))
	case DiffMsg:
		m.apply(rpc.Diff(msg))
	case TickMsg:
		m.frame++
		return m, tea.Batch(m.tick(), m.tickDisk())
	case diskMsg:
		m.gotDisk(msg)
	case diskDoneMsg:
		return m, m.gotDiskAction(msg)
	case diskShellMsg:
		if msg.err != nil {
			m.dk.status = msg.err.Error()
			break
		}
		return m.closeDisk()
	case TopBarMsg:
		m.top = msg
	case DisconnectedMsg:
		m.status = "daemon disconnected"
	case errMsg:
		m.status = msg.err.Error()
	case reviewMsg:
		m.gotReview(msg)
	case tea.KeyPressMsg:
		if m.dialog != nil {
			return m.dialogKey(msg)
		}
		if m.rv.open {
			return m.reviewKey(msg.String())
		}
		if m.renaming != nil {
			return m.renameKey(msg)
		}
		if m.dk.open {
			return m.diskKey(msg.String())
		}
		return m.key(msg)
	case tea.PasteMsg:
		if m.dialog != nil {
			return m.dialogPaste(msg.Content), nil
		}
		if m.renaming != nil {
			return m.renamePaste(msg.Content), nil
		}
	case sessionStartedMsg:
		if m.dialog != nil && m.dialog.seq == msg.seq {
			m.dialog = nil
		}
		m.pending = msg.session.ID
		m.choosePending()
		return m, m.showNew(msg.session.ID)
	case startFailedMsg:
		if m.dialog == nil || m.dialog.seq != msg.seq {
			m.status = msg.err.Error()
			break
		}
		d := m.own()
		d.busy, d.err = false, msg.err.Error()
	}
	return m, nil
}

func (m *Model) load(st rpc.State) {
	m.workspaces = map[string]domain.Workspace{}
	for _, w := range st.Workspaces {
		m.workspaces[w.Root] = w
	}
	m.tasks = map[string]domain.Task{}
	m.worktrees = map[string]domain.Worktree{}
	m.sessions = map[string]domain.Session{}
	m.events = map[string][]domain.SessionEvent{}
	m.subagents = map[string][]domain.Subagent{}
	m.taskOrder = nil
	for _, t := range st.Tasks {
		m.putTask(t)
	}
	for _, w := range st.Worktrees {
		m.worktrees[w.ID] = w
	}
	for _, s := range st.Sessions {
		m.sessions[s.ID] = s
	}
	for _, ev := range st.Events {
		m.addEvent(ev)
	}
	for _, sub := range st.Subagents {
		m.putSubagent(sub)
	}
	m.rebuild()
}

func (m *Model) addEvent(ev domain.SessionEvent) {
	kept := append(m.events[ev.SessionID], ev)
	if len(kept) > domain.SessionEventsKept {
		kept = kept[len(kept)-domain.SessionEventsKept:]
	}
	m.events[ev.SessionID] = kept
}

func (m *Model) apply(d rpc.Diff) {
	if d.Event != nil {
		m.addEvent(*d.Event)
	}
	switch {
	case d.Workspace != nil:
		m.workspaces[d.Workspace.Root] = *d.Workspace
		return
	case d.RemovedWorkspace != "":
		delete(m.workspaces, d.RemovedWorkspace)
		return
	case d.Task != nil:
		m.putTask(*d.Task)
	case d.RemovedWorktree != "":
		delete(m.worktrees, d.RemovedWorktree)
	case d.Worktree != nil:
		m.worktrees[d.Worktree.ID] = *d.Worktree
	case d.Session != nil:
		m.sessions[d.Session.ID] = *d.Session
	case d.Subagent != nil:
		m.putSubagent(*d.Subagent)
		return
	default:
		return
	}
	m.rebuild()
}

func (m *Model) putTask(t domain.Task) {
	if _, ok := m.tasks[t.ID]; !ok {
		m.taskOrder = append(m.taskOrder, t.ID)
	}
	m.tasks[t.ID] = t
}

func (m *Model) rebuild() {
	tasks := make([]domain.Task, 0, len(m.taskOrder))
	for _, id := range m.taskOrder {
		tasks = append(tasks, m.tasks[id])
	}
	sessions := make([]domain.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ID < sessions[j].ID })

	m.entries = make([]entry, 0, len(sessions))
	for _, g := range domain.Sidebar(tasks, sessions) {
		var repos []string
		seen := map[string]bool{}
		start := len(m.entries)
		for _, s := range g.Sessions {
			e := entry{session: s, task: g.Task, num: len(m.entries) + 1}
			for _, id := range s.WorktreeIDs {
				w, ok := m.worktrees[id]
				if !ok {
					continue
				}
				e.worktrees = append(e.worktrees, w)
				if !seen[w.Repo] {
					seen[w.Repo] = true
					repos = append(repos, w.Repo)
				}
			}
			m.entries = append(m.entries, e)
		}
		m.entries[start].groupStart = true
		m.entries[start].groupRepos = repos
	}
	if m.index(m.selected) < 0 {
		m.selected = ""
		if len(m.entries) > 0 {
			m.selected = m.entries[0].session.ID
		}
	}
	if m.index(m.last) < 0 {
		m.last = ""
	}
	m.choosePending()
}

func (m *Model) choosePending() {
	if i := m.index(m.pending); i >= 0 {
		m.choose(i)
		m.pending = ""
	}
}

func (m Model) index(id string) int {
	if id == "" {
		return -1
	}
	for i, e := range m.entries {
		if e.session.ID == id {
			return i
		}
	}
	return -1
}

func (m *Model) choose(i int) {
	if i < 0 || i >= len(m.entries) {
		return
	}
	id := m.entries[i].session.ID
	if id == m.selected {
		return
	}
	m.last, m.selected = m.selected, id
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if prompt := m.confirm; prompt != nil {
		m.confirm = nil
		if k == "y" {
			return m, m.kill(prompt.pgids)
		}
		return m, nil
	}
	if m.picker != nil {
		return m.pickerKey(k)
	}
	cur := m.index(m.selected)
	if m.ending != "" {
		id := m.ending
		m.ending, m.status = "", ""
		if k == "y" {
			return m, m.endSession(id)
		}
		return m, nil
	}
	switch k {
	case "n":
		if m.opts.Calls != nil {
			return m.openDialog(), nil
		}
	case "x":
		if cur >= 0 && m.opts.Calls != nil {
			m.ending = m.selected
			m.status = fmt.Sprintf("end session %d? y/n", m.entries[cur].num)
		}
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.help = !m.help
	case "M":
		m = m.openPicker(domain.SwitchModel)
	case "E":
		m = m.openPicker(domain.SwitchEffort)
	case "j", "down":
		m.choose(cur + 1)
	case "k", "up":
		m.choose(cur - 1)
	case "tab":
		m.choose(m.index(m.last))
	case "space":
		m.choose(m.nextNeedingYou(cur))
	case "o":
		if m.selected != "" {
			m.collapsed[m.selected] = !m.collapsed[m.selected]
		}
	case "R":
		return m.askRename(), nil
	case "A":
		return m, m.unpin()
	case "m":
		return m, m.toggleMute()
	case "K":
		m.askKill()
	case "enter":
		return m, m.focus()
	case "r":
		return m.openReview()
	case "w":
		return m.openDisk()
	default:
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			m.choose(int(k[0] - '1'))
		}
	}
	return m, nil
}

func (m Model) nextNeedingYou(cur int) int {
	n := len(m.entries)
	for step := 1; step <= n; step++ {
		i := (cur + step + n) % n
		if m.entries[i].session.NeedsYou() {
			return i
		}
	}
	return -1
}

func (m Model) focus() tea.Cmd {
	f := m.opts.Focus
	if f == nil || m.selected == "" {
		return nil
	}
	id, attend := m.selected, m.opts.Attend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := f.FocusMain(ctx); err != nil {
			return errMsg{err}
		}
		if attend != nil {
			if err := attend.FocusSession(ctx, id); err != nil {
				return errMsg{err}
			}
		}
		return nil
	}
}

func (m Model) toggleMute() tea.Cmd {
	a := m.opts.Attend
	x, ok := m.sessions[m.selected]
	if a == nil || !ok {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := a.MuteSession(ctx, x.ID, !x.Muted); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m *Model) askKill() {
	i := m.index(m.selected)
	if i < 0 || m.opts.Kill == nil {
		return
	}
	ports := m.entries[i].ports()
	if len(ports) == 0 {
		return
	}
	m.confirm = &killPrompt{pgids: groupsOf(ports), label: portLabel(ports)}
}

func (m Model) kill(pgids []int) tea.Cmd {
	k := m.opts.Kill
	if k == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), killTimeout)
		defer cancel()
		if _, err := k.KillPorts(ctx, pgids); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

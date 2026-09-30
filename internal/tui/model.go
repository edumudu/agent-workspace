// Package tui is the Bubble Tea client. It renders only from the daemon's
// state snapshot and diffs, and never runs commands or touches disk while
// rendering.
package tui

import (
	"context"
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

type Options struct {
	Theme Theme
	// Now is the clock for the top bar; nil means time.Now.
	Now func() time.Time
	// Tick is the spinner and clock interval; zero turns the ticker off.
	Tick  time.Duration
	Focus Focuser
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

// DisconnectedMsg reports that the daemon connection ended.
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

	tasks     map[string]domain.Task
	taskOrder []string
	worktrees map[string]domain.Worktree
	sessions  map[string]domain.Session
	entries   []entry
	collapsed map[string]bool

	selected string
	last     string
	help     bool
	frame    int
	top      TopBarMsg
	status   string
}

func New(opts Options) Model {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return Model{
		opts:      opts,
		styles:    newStyles(opts.Theme),
		width:     40,
		height:    24,
		tasks:     map[string]domain.Task{},
		worktrees: map[string]domain.Worktree{},
		sessions:  map[string]domain.Session{},
		collapsed: map[string]bool{},
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
		return m, m.tick()
	case TopBarMsg:
		m.top = msg
	case DisconnectedMsg:
		m.status = "daemon disconnected"
	case errMsg:
		m.status = msg.err.Error()
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) load(st rpc.State) {
	m.tasks = map[string]domain.Task{}
	m.worktrees = map[string]domain.Worktree{}
	m.sessions = map[string]domain.Session{}
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
	m.rebuild()
}

func (m *Model) apply(d rpc.Diff) {
	switch {
	case d.Task != nil:
		m.putTask(*d.Task)
	case d.Worktree != nil:
		m.worktrees[d.Worktree.ID] = *d.Worktree
	case d.Session != nil:
		m.sessions[d.Session.ID] = *d.Session
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
	cur := m.index(m.selected)
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.help = !m.help
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
	case "enter":
		return m, m.focus()
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
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := f.FocusMain(ctx); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

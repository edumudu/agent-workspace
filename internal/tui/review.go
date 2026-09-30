package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// Reviewer builds reviews and widens the layout for them; *rpc.Client is one.
type Reviewer interface {
	Review(ctx context.Context, p rpc.ReviewParams) (rpc.Review, error)
	MarkViewed(ctx context.Context, mark domain.ViewedMark, viewed bool) error
	ReviewLayout(ctx context.Context, open bool) error
}

const reviewCallTimeout = 15 * time.Second

type styledLine struct {
	line domain.DiffLine
	toks []tok
}

// diffRow is one row of a file's diff: a hunk header, or a line (unified)
// or a pair of lines (split, either side may be nil).
type diffRow struct {
	header string
	one    *styledLine
	left   *styledLine
	right  *styledLine
}

type reviewFile struct {
	wt      domain.Worktree
	file    domain.FileDiff
	unified []diffRow
	split   []diffRow
}

// treeRow is a worktree heading when file is -1, else an index into files.
type treeRow struct {
	wt   domain.Worktree
	err  string
	file int
}

// prepared is a review ready to draw: parsed, highlighted and laid out in
// the command that fetched it, so View never lexes.
type prepared struct {
	files []reviewFile
	tree  []treeRow
}

type reviewMsg struct {
	seq    int
	review rpc.Review
	prep   prepared
	err    error
}

type reviewState struct {
	open     bool
	session  string
	scope    domain.ReviewScope
	worktree string
	split    bool
	seq      int
	loading  bool
	err      string

	review rpc.Review
	prep   prepared
	marks  map[string]domain.ViewedMark
	cur    int
	scroll int
}

func prepare(syntax syntaxColors, r rpc.Review) prepared {
	var p prepared
	for _, w := range r.Worktrees {
		p.tree = append(p.tree, treeRow{wt: w.Worktree, err: w.Err, file: -1})
		for _, f := range w.Files {
			p.tree = append(p.tree, treeRow{wt: w.Worktree, file: len(p.files)})
			p.files = append(p.files, layoutFile(syntax, w.Worktree, f))
		}
	}
	return p
}

func layoutFile(syntax syntaxColors, wt domain.Worktree, f domain.FileDiff) reviewFile {
	rf := reviewFile{wt: wt, file: f}
	var all []domain.DiffLine
	for _, h := range f.Hunks {
		all = append(all, h.Lines...)
	}
	toks := highlight(syntax, f.Path, all)
	styled := map[domain.DiffLine]*styledLine{}
	i := 0
	for _, h := range f.Hunks {
		rf.unified = append(rf.unified, diffRow{header: h.Header})
		rf.split = append(rf.split, diffRow{header: h.Header})
		for _, l := range h.Lines {
			sl := &styledLine{line: l, toks: toks[i]}
			styled[l] = sl
			i++
			rf.unified = append(rf.unified, diffRow{one: sl})
		}
		for _, r := range domain.SplitRows(h) {
			row := diffRow{}
			if r.Left != nil {
				row.left = styled[*r.Left]
			}
			if r.Right != nil {
				row.right = styled[*r.Right]
			}
			rf.split = append(rf.split, row)
		}
	}
	return rf
}

func (m Model) openReview() (tea.Model, tea.Cmd) {
	r := m.opts.Review
	if r == nil || m.selected == "" {
		return m, nil
	}
	if m.rv.scope == "" {
		m.rv.scope = domain.ScopeUncommitted
	}
	if m.rv.session != m.selected {
		m.rv.worktree, m.rv.cur, m.rv.scroll = "", 0, 0
	}
	m.rv.open, m.rv.session = true, m.selected
	fetch := m.fetchReview()
	return m, tea.Batch(layout(r, true), fetch)
}

func (m Model) closeReview() (tea.Model, tea.Cmd) {
	m.rv.open = false
	return m, layout(m.opts.Review, false)
}

func layout(r layouter, open bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := r.ReviewLayout(ctx, open); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

// fetchReview asks for the review with the current settings; the answer
// carries the seq it was asked with, and one that is no longer current is
// dropped.
func (m *Model) fetchReview() tea.Cmd {
	m.rv.seq++
	m.rv.loading = true
	r, seq, syntax := m.opts.Review, m.rv.seq, newSyntaxColors(m.opts.Theme)
	p := rpc.ReviewParams{Session: m.rv.session, Scope: m.rv.scope, Worktree: m.rv.worktree}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), reviewCallTimeout)
		defer cancel()
		rev, err := r.Review(ctx, p)
		if err != nil {
			return reviewMsg{seq: seq, err: err}
		}
		return reviewMsg{seq: seq, review: rev, prep: prepare(syntax, rev)}
	}
}

func (m *Model) gotReview(msg reviewMsg) {
	if msg.seq != m.rv.seq {
		return
	}
	m.rv.loading = false
	if msg.err != nil {
		m.rv.err = msg.err.Error()
		m.rv.review, m.rv.prep = rpc.Review{Scope: m.rv.scope}, prepared{}
		m.rv.cur, m.rv.scroll = 0, 0
		return
	}
	prevPath := ""
	if f, ok := m.rv.current(); ok {
		prevPath = f.wt.ID + "\x00" + f.file.Path
	}
	m.rv.err, m.rv.review, m.rv.prep = "", msg.review, msg.prep
	m.rv.marks = map[string]domain.ViewedMark{}
	for _, mk := range msg.review.Viewed {
		m.rv.marks[mk.Key()] = mk
	}
	m.rv.cur, m.rv.scroll = 0, 0
	for i, f := range m.rv.prep.files {
		if f.wt.ID+"\x00"+f.file.Path == prevPath {
			m.rv.cur = i
		}
	}
}

func (r reviewState) current() (reviewFile, bool) {
	if r.cur < 0 || r.cur >= len(r.prep.files) {
		return reviewFile{}, false
	}
	return r.prep.files[r.cur], true
}

func (r reviewState) viewed(f reviewFile) bool {
	return domain.IsViewed(r.marks, f.wt.ID, f.file)
}

func (m Model) reviewKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r", "esc":
		return m.closeReview()
	case "]", "[", "w":
		switch k {
		case "]":
			m.rv.scope = m.rv.scope.Shift(1)
		case "[":
			m.rv.scope = m.rv.scope.Shift(-1)
		default:
			m.rv.worktree = m.nextWorktree()
		}
		fetch := m.fetchReview()
		return m, fetch
	case "u":
		m.rv.split = !m.rv.split
		m.rv.scroll = 0
	case "v":
		mark := m.toggleViewed()
		return m, mark
	case "n":
		m.moveFile(1)
	case "p":
		m.moveFile(-1)
	case "j", "down":
		m.scrollBy(1)
	case "k", "up":
		m.scrollBy(-1)
	case "ctrl+d":
		m.scrollBy(m.diffHeight() / 2)
	case "ctrl+u":
		m.scrollBy(-m.diffHeight() / 2)
	}
	return m, nil
}

// nextWorktree cycles all, then each of the session's worktrees.
func (m Model) nextWorktree() string {
	ids := []string{""}
	for _, e := range m.entries {
		if e.session.ID == m.rv.session {
			for _, w := range e.worktrees {
				ids = append(ids, w.ID)
			}
		}
	}
	for i, id := range ids {
		if id == m.rv.worktree {
			return ids[(i+1)%len(ids)]
		}
	}
	return ""
}

func (m *Model) moveFile(delta int) {
	n := len(m.rv.prep.files)
	if n == 0 {
		return
	}
	next := min(max(m.rv.cur+delta, 0), n-1)
	if next != m.rv.cur {
		m.rv.cur, m.rv.scroll = next, 0
	}
}

func (m *Model) scrollBy(delta int) {
	f, ok := m.rv.current()
	if !ok {
		return
	}
	rows := len(f.unified)
	if m.rv.split {
		rows = len(f.split)
	}
	m.rv.scroll = min(max(m.rv.scroll+delta, 0), max(rows-m.diffHeight(), 0))
}

func (m *Model) toggleViewed() tea.Cmd {
	f, ok := m.rv.current()
	if !ok {
		return nil
	}
	mark := domain.ViewedMark{Worktree: f.wt.ID, Path: f.file.Path, Blob: f.file.Blob}
	viewed := !m.rv.viewed(f)
	marks := make(map[string]domain.ViewedMark, len(m.rv.marks)+1)
	for k, v := range m.rv.marks {
		marks[k] = v
	}
	if viewed {
		marks[mark.Key()] = mark
	} else {
		delete(marks, mark.Key())
	}
	m.rv.marks = marks
	r := m.opts.Review
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := r.MarkViewed(ctx, mark, viewed); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

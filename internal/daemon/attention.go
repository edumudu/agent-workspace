package daemon

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const (
	// why: more are dropped rather than stall the loop.
	bannerQueue   = 64
	bannerTimeout = 5 * time.Second
)

type queuedBanner struct {
	banner  domain.Banner
	focused bool
	// why: set instead of banner to withdraw that group's banner.
	remove string
}

// why: decides on the loop and delivers on a worker: the notifier and the
// frontmost check both run processes.
type attention struct {
	notifier app.Notifier
	fg       app.Foreground
	sounds   map[domain.AgentState]string
	co       *domain.Coalescer
	queue    chan queuedBanner
	// why: written by client.open on a connection goroutine, read by the worker.
	terminal atomic.Pointer[string]
	// why: owned by the loop; only sessions with a banner up are withdrawn, so a focus costs no process otherwise.
	posted map[string]bool
}

// why: fg may be nil, which counts as the terminal never being in front.
func WithNotifier(n app.Notifier, fg app.Foreground, sounds map[domain.AgentState]string) Option {
	return func(d *Daemon) {
		d.st.attn = &attention{
			notifier: n,
			fg:       fg,
			sounds:   sounds,
			co:       domain.NewCoalescer(),
			queue:    make(chan queuedBanner, bannerQueue),
			posted:   map[string]bool{},
		}
	}
}

func (a *attention) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case q := <-a.queue:
			a.deliver(ctx, q)
		}
	}
}

func (a *attention) deliver(ctx context.Context, q queuedBanner) {
	ctx, cancel := context.WithTimeout(ctx, bannerTimeout)
	defer cancel()
	if q.remove != "" {
		if err := a.notifier.Remove(ctx, q.remove); err != nil {
			log.Printf("notify remove: %v", err)
		}
		return
	}
	if t := a.terminal.Load(); t != nil {
		q.banner.Terminal = *t
	}
	if q.focused && a.fg != nil && a.fg.TerminalFrontmost(ctx) {
		return
	}
	if err := a.notifier.Notify(ctx, q.banner); err != nil {
		log.Printf("notify: %v", err)
	}
}

func (s *state) announce(session domain.Session, effects []domain.Effect) {
	if s.attn == nil {
		return
	}
	name := s.sessionName(session)
	var worktrees []domain.Worktree
	for _, id := range session.WorktreeIDs {
		if w, ok := s.worktrees[id]; ok {
			worktrees = append(worktrees, w)
		}
	}
	for _, e := range effects {
		b, ok := domain.BannerFor(domain.BannerInput{
			Session: session, Name: name, Effect: e,
			Worktrees: worktrees, Events: s.events[session.ID], Now: time.Now(),
		})
		if !ok || !s.attn.co.Allow(session.ID, time.Now()) {
			continue
		}
		b.Sound = s.attn.sounds[b.State]
		b.Group = session.ID
		select {
		case s.attn.queue <- queuedBanner{banner: b, focused: session.Focused}:
			s.attn.posted[session.ID] = true
		default:
		}
	}
}

func (s *state) withdraw(id string) {
	if s.attn == nil || !s.attn.posted[id] {
		return
	}
	select {
	case s.attn.queue <- queuedBanner{remove: id}:
		delete(s.attn.posted, id)
	default:
	}
}

func (a *attention) setTerminal(bundle string) {
	if a != nil && bundle != "" {
		a.terminal.Store(&bundle)
	}
}

func (s *state) sessionName(session domain.Session) string {
	var prs []domain.PullRequest
	for _, id := range session.WorktreeIDs {
		if w, ok := s.worktrees[id]; ok && w.PR != nil {
			prs = append(prs, *w.PR)
		}
	}
	return domain.NameFor(s.tasks[session.TaskID], prs)
}

func (d *Daemon) dispatchAttention(req rpc.Request) (*rpc.Response, bool) {
	var id string
	var apply func(s *state, session domain.Session)
	switch req.Method {
	case rpc.MethodSessionMute:
		var p rpc.SessionMuteParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errorResponse(req.ID, rpc.CodeBadRequest, "session.mute params: "+err.Error()), true
		}
		id = p.ID
		apply = func(s *state, session domain.Session) { s.emit(SessionChanged{Session: session.SetMuted(p.Muted)}) }
	default:
		var p rpc.SessionFocusParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errorResponse(req.ID, rpc.CodeBadRequest, "session.focus params: "+err.Error()), true
		}
		id = p.ID
		apply = func(s *state, session domain.Session) { s.focus(session) }
	}
	found := false
	ok := d.query(func(s *state) {
		session, exists := s.sessions[id]
		if !exists {
			return
		}
		found = true
		apply(s, session)
	})
	if ok && !found {
		return errorResponse(req.ID, rpc.CodeNotFound, "no session "+id), true
	}
	return result(req.ID, struct{}{}), ok
}

func (s *state) focus(session domain.Session) {
	s.withdraw(session.ID)
	for _, other := range sorted(s.sessions) {
		if other.ID != session.ID && other.Focused {
			s.emit(SessionChanged{Session: other.Blur()})
		}
	}
	if !session.Focused || session.Unread {
		s.emit(SessionChanged{Session: session.Focus()})
	}
}

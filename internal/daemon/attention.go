package daemon

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const (
	// bannerQueue is how many banners may wait for the worker; more are dropped
	// rather than stall the loop.
	bannerQueue   = 64
	bannerTimeout = 5 * time.Second
)

type queuedBanner struct {
	banner  domain.Banner
	focused bool
}

// attention decides on the loop and delivers on a worker: the notifier and
// the frontmost check both run processes.
type attention struct {
	notifier app.Notifier
	fg       app.Foreground
	sounds   map[domain.AgentState]string
	co       *domain.Coalescer
	queue    chan queuedBanner
}

// WithNotifier turns on banners. fg may be nil, which counts as the terminal
// never being in front. sounds names a macOS sound per event state.
func WithNotifier(n app.Notifier, fg app.Foreground, sounds map[domain.AgentState]string) Option {
	return func(d *Daemon) {
		d.st.attn = &attention{
			notifier: n,
			fg:       fg,
			sounds:   sounds,
			co:       domain.NewCoalescer(),
			queue:    make(chan queuedBanner, bannerQueue),
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
	if q.focused && a.fg != nil && a.fg.TerminalFrontmost(ctx) {
		return
	}
	if err := a.notifier.Notify(ctx, q.banner); err != nil {
		log.Printf("notify: %v", err)
	}
}

// announce queues a banner for each notify effect that survives mute and
// coalescing. It runs on the loop and never blocks.
func (s *state) announce(session domain.Session, effects []domain.Effect) {
	if s.attn == nil {
		return
	}
	name := s.sessionName(session)
	for _, e := range effects {
		b, ok := domain.BannerFor(session, name, e)
		if !ok || !s.attn.co.Allow(session.ID, time.Now()) {
			continue
		}
		b.Sound = s.attn.sounds[b.State]
		select {
		case s.attn.queue <- queuedBanner{banner: b, focused: session.Focused}:
		default:
		}
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

// focus makes session the one in view. Only sessions that change are emitted.
func (s *state) focus(session domain.Session) {
	for _, other := range sorted(s.sessions) {
		if other.ID != session.ID && other.Focused {
			s.emit(SessionChanged{Session: other.Blur()})
		}
	}
	if !session.Focused || session.Unread {
		s.emit(SessionChanged{Session: session.Focus()})
	}
}

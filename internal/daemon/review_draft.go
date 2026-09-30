package daemon

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const sendDraftTimeout = 5 * time.Second

// WithHunks enables review.hunk, which stages or reverts one hunk.
func WithHunks(h app.HunkGit) Option {
	return func(d *Daemon) { d.rv.hunks = h }
}

// reviewTarget is the first worktree session reviews that match accepts.
func (s *state) reviewTarget(session string, match func(domain.Worktree) bool) (domain.Worktree, bool) {
	for _, t := range s.reviewTargets(session) {
		if match(t.Worktree) {
			return t.Worktree, true
		}
	}
	return domain.Worktree{}, false
}

func (d *Daemon) addDraftComment(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.ReviewCommentParams
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Session == "" || p.Comment.Path == "" || p.Comment.Body == "" {
		return errorResponse(req.ID, rpc.CodeBadRequest, "review.comment needs a session, a path and a body"), true
	}
	var resp *rpc.Response
	ok := d.query(func(s *state) {
		if _, found := s.sessions[p.Session]; !found {
			resp = errorResponse(req.ID, rpc.CodeNotFound, "no session "+p.Session)
			return
		}
		if _, owned := s.reviewTarget(p.Session, func(w domain.Worktree) bool { return w.Path == p.Comment.Worktree }); !owned {
			resp = errorResponse(req.ID, rpc.CodeBadRequest, "session "+p.Session+" does not work in "+p.Comment.Worktree)
			return
		}
		draft, open := s.drafts[p.Session]
		if !open {
			draft = domain.ReviewDraft{ID: p.Session + "-" + strconv.FormatInt(time.Now().UnixNano(), 10), Session: p.Session}
		}
		draft = draft.Add(p.Comment)
		s.drafts[p.Session] = draft
		s.store.PutDraft(draft)
		resp = result(req.ID, draft)
	})
	return resp, ok
}

func (d *Daemon) sendReview(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.ReviewSendParams
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Session == "" {
		return errorResponse(req.ID, rpc.CodeBadRequest, "review.send needs a session"), true
	}
	if d.hs.host == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "no terminal host"), true
	}
	var resp *rpc.Response
	ok := d.query(func(s *state) {
		session, found := s.sessions[p.Session]
		if !found {
			resp = errorResponse(req.ID, rpc.CodeNotFound, "no session "+p.Session)
			return
		}
		draft, open := s.drafts[p.Session]
		if !open || len(draft.Comments) == 0 {
			resp = errorResponse(req.ID, rpc.CodeBadRequest, "the draft has no comments")
			return
		}
		draft = draft.Queue()
		s.drafts[p.Session] = draft
		s.store.PutDraft(draft)
		if sent, ok := s.dispatchDraft(session); ok {
			draft = sent
		}
		resp = result(req.ID, draft)
	})
	return resp, ok
}

// dispatchDraft sends session's queued draft if it is between tools,
// archiving it as sent; the paste itself runs on a worker.
func (s *state) dispatchDraft(session domain.Session) (domain.ReviewDraft, bool) {
	draft, ok := s.drafts[session.ID]
	if !ok || s.sendDraft == nil {
		return draft, false
	}
	sent, prompt, ok := draft.Dispatch(session, time.Now())
	if !ok {
		return draft, false
	}
	delete(s.drafts, session.ID)
	s.awaiting[session.ID] = sent
	s.store.PutDraft(sent)
	s.sendDraft(session, sent, prompt)
	return sent, true
}

// sendDraft pastes the prompt on a worker, taking turns with switches for
// the pane. If the session started a turn meanwhile, or the paste fails, the
// draft goes back to the queue.
func (d *Daemon) sendDraft(session domain.Session, draft domain.ReviewDraft, prompt string) {
	go func() {
		d.hs.sendMu.Lock()
		defer d.hs.sendMu.Unlock()
		ready := false
		d.query(func(s *state) {
			current, ok := s.sessions[session.ID]
			ready = ok && current.AcceptsSwitch()
		})
		if ready {
			ctx, cancel := context.WithTimeout(context.Background(), sendDraftTimeout)
			err := app.SendPrompt(ctx, d.hs.host, app.PaneID(session.Pane), prompt, app.PasteSettle)
			cancel()
			if err == nil {
				return
			}
		}
		d.query(func(s *state) { s.requeueDraft(draft) })
	}()
}

// requeueDraft puts back a draft that was not pasted, ahead of any comments
// added since.
func (s *state) requeueDraft(draft domain.ReviewDraft) {
	if s.awaiting[draft.Session].ID == draft.ID {
		delete(s.awaiting, draft.Session)
	}
	back := draft.Unsend()
	if newer, ok := s.drafts[draft.Session]; ok {
		s.store.PutDraft(domain.ReviewDraft{ID: newer.ID, Session: newer.Session, Status: domain.DraftMerged})
		back.Comments = append(back.Comments, newer.Comments...)
	}
	s.drafts[draft.Session] = back
	s.store.PutDraft(back)
}

func (d *Daemon) applyHunk(req rpc.Request) (*rpc.Response, bool) {
	var p rpc.HunkParams
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Session == "" || p.Worktree == "" {
		return errorResponse(req.ID, rpc.CodeBadRequest, "review.hunk needs a session and a worktree"), true
	}
	if d.rv.hunks == nil {
		return errorResponse(req.ID, rpc.CodeUnavailable, "hunk staging is not enabled"), true
	}
	if p.Hunk < 0 || p.Hunk >= len(p.File.Hunks) {
		return errorResponse(req.ID, rpc.CodeBadRequest, "no hunk "+strconv.Itoa(p.Hunk)+" in "+p.File.Path), true
	}
	var wt domain.Worktree
	owned := false
	ok := d.query(func(s *state) {
		wt, owned = s.reviewTarget(p.Session, func(w domain.Worktree) bool { return w.ID == p.Worktree })
	})
	if !owned {
		return errorResponse(req.ID, rpc.CodeBadRequest, "session "+p.Session+" does not work in "+p.Worktree), ok
	}
	ctx, cancel := context.WithTimeout(context.Background(), reviewTimeout)
	defer cancel()
	if err := app.ApplyHunk(ctx, d.rv.hunks, wt.Path, p.File, p.File.Hunks[p.Hunk], p.Action); err != nil {
		return errorResponse(req.ID, rpc.CodeFailed, err.Error()), ok
	}
	return result(req.ID, struct{}{}), ok
}

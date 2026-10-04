package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const (
	CloseUnauthorized  websocket.StatusCode = 4401
	streamReadLimit                         = 64 << 10
	streamWriteTimeout                      = 10 * time.Second
)

var (
	errClientGone = errors.New("client gone")
	errDaemonGone = errors.New("the agentws daemon went away")
)

type StreamState struct {
	Seq        uint64              `json:"seq"`
	Workspaces []domain.Workspace  `json:"workspaces"`
	Tasks      []domain.Task       `json:"tasks"`
	Worktrees  []domain.Worktree   `json:"worktrees"`
	Sessions   []domain.Session    `json:"sessions"`
	Queue      []domain.LaunchItem `json:"queue"`
	Sends      []domain.QueuedSend `json:"sends"`
}

type StreamDiff struct {
	Seq              uint64               `json:"seq"`
	RemovedWorkspace string               `json:"removed_workspace,omitempty"`
	RemovedWorktree  string               `json:"removed_worktree,omitempty"`
	RemovedSession   string               `json:"removed_session,omitempty"`
	Workspace        *domain.Workspace    `json:"workspace,omitempty"`
	Task             *domain.Task         `json:"task,omitempty"`
	Worktree         *domain.Worktree     `json:"worktree,omitempty"`
	Session          *domain.Session      `json:"session,omitempty"`
	Queue            *[]domain.LaunchItem `json:"queue,omitempty"`
	Sends            *[]domain.QueuedSend `json:"sends,omitempty"`
}

type Frame struct {
	State *StreamState `json:"state,omitempty"`
	Diff  *StreamDiff  `json:"diff,omitempty"`
	Error *rpc.Error   `json:"error,omitempty"`
}

type clientFrame struct {
	Token string `json:"token"`
}

type stream struct {
	conn    *websocket.Conn
	readCtx context.Context
	writeMu sync.Mutex
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if err := s.checkOrigin(r); err != nil {
		writeError(w, err)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(streamReadLimit)
	ctx, cancel := context.WithCancelCause(s.base)
	defer cancel(nil)
	readCtx, stopReading := context.WithCancel(context.Background())
	defer stopReading()
	st := &stream{conn: conn, readCtx: readCtx}
	code, reason := s.runStream(ctx, cancel, st)
	_ = conn.Close(code, reason)
}

func (s *Server) checkOrigin(r *http.Request) error {
	if s.origin == "" {
		return &rpc.Error{Code: codeForbidden, Message: "agentws serve has no public URL; set [serve] url in config.toml or pass --url"}
	}
	origin, err := originOf(r.Header.Get("Origin"))
	if err != nil || origin != s.origin {
		return &rpc.Error{Code: codeForbidden, Message: "the stream only accepts the Origin " + s.origin}
	}
	return nil
}

func (s *Server) runStream(ctx context.Context, cancel context.CancelCauseFunc, st *stream) (websocket.StatusCode, string) {
	d, device, code, reason := s.authenticate(ctx, st)
	if d == nil {
		return code, reason
	}
	defer func() { _ = d.Close() }()
	unregister, ok := s.streams.add(device.ID, cancel)
	if !ok {
		return CloseUnauthorized, errRevoked.Error()
	}
	defer unregister()
	sub, err := d.Subscribe(ctx)
	if err != nil {
		return websocket.StatusInternalError, apiError(err).Message
	}
	if err := st.write(ctx, Frame{State: filterState(sub.State)}); err != nil {
		return websocket.StatusInternalError, "write failed"
	}
	go func() {
		cancel(st.readLoop(st.readCtx, ctx))
	}()
	for {
		select {
		case <-ctx.Done():
			return closeFor(context.Cause(ctx))
		case diff, ok := <-sub.Diffs:
			if !ok {
				return websocket.StatusTryAgainLater, errDaemonGone.Error()
			}
			out, keep := filterDiff(diff)
			if !keep {
				continue
			}
			if err := st.write(ctx, Frame{Diff: out}); err != nil {
				return websocket.StatusInternalError, "write failed"
			}
		}
	}
}

func (s *Server) authenticate(ctx context.Context, st *stream) (Daemon, rpc.Device, websocket.StatusCode, string) {
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	late := time.AfterFunc(s.cfg.AuthTimeout, func() {
		_ = st.conn.Close(CloseUnauthorized, "no token within "+s.cfg.AuthTimeout.String())
	})
	_, data, err := st.conn.Read(readCtx)
	if !late.Stop() {
		return nil, rpc.Device{}, CloseUnauthorized, "no token in time"
	}
	if err != nil {
		return nil, rpc.Device{}, websocket.StatusNormalClosure, errClientGone.Error()
	}
	var first clientFrame
	if json.Unmarshal(data, &first) != nil || first.Token == "" {
		return nil, rpc.Device{}, CloseUnauthorized, `the first frame must be {"token": "<device token>"}`
	}
	d, err := s.dial(ctx)
	if err != nil {
		return nil, rpc.Device{}, websocket.StatusTryAgainLater, apiError(err).Message
	}
	device, err := s.check(ctx, d, first.Token)
	if err != nil {
		_ = d.Close()
		e := apiError(err)
		if e.Code == rpc.CodeUnauthorized {
			return nil, rpc.Device{}, CloseUnauthorized, e.Message
		}
		return nil, rpc.Device{}, websocket.StatusTryAgainLater, e.Message
	}
	return d, device, 0, ""
}

func (st *stream) readLoop(readCtx, ctx context.Context) error {
	for {
		_, data, err := st.conn.Read(readCtx)
		if err != nil {
			return errClientGone
		}
		if err := st.handle(ctx, data); err != nil {
			return err
		}
	}
}

func (st *stream) handle(ctx context.Context, data []byte) error {
	var f map[string]json.RawMessage
	if json.Unmarshal(data, &f) != nil {
		return st.write(ctx, Frame{Error: &rpc.Error{Code: rpc.CodeBadRequest, Message: "frames are JSON objects"}})
	}
	return st.write(ctx, Frame{Error: &rpc.Error{Code: rpc.CodeBadRequest, Message: "unknown frame"}})
}

func (st *stream) write(ctx context.Context, f Frame) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	st.writeMu.Lock()
	defer st.writeMu.Unlock()
	wctx, cancel := context.WithTimeout(ctx, streamWriteTimeout)
	defer cancel()
	return st.conn.Write(wctx, websocket.MessageText, data)
}

func closeFor(cause error) (websocket.StatusCode, string) {
	switch {
	case errors.Is(cause, errRevoked):
		return CloseUnauthorized, errRevoked.Error()
	case errors.Is(cause, errClientGone):
		return websocket.StatusNormalClosure, ""
	case errors.Is(cause, context.Canceled):
		return websocket.StatusGoingAway, "agentws serve is stopping"
	default:
		return websocket.StatusInternalError, cause.Error()
	}
}

func filterState(st rpc.State) *StreamState {
	worktrees := make([]domain.Worktree, 0, len(st.Worktrees))
	for _, wt := range st.Worktrees {
		worktrees = append(worktrees, withoutPorts(wt))
	}
	return &StreamState{
		Seq:        st.Seq,
		Workspaces: orEmpty(st.Workspaces),
		Tasks:      orEmpty(st.Tasks),
		Worktrees:  worktrees,
		Sessions:   orEmpty(st.Sessions),
		Queue:      orEmpty(st.Queue),
		Sends:      orEmpty(st.Sends),
	}
}

func filterDiff(d rpc.Diff) (*StreamDiff, bool) {
	out := &StreamDiff{
		Seq:              d.Seq,
		RemovedWorkspace: d.RemovedWorkspace,
		RemovedWorktree:  d.RemovedWorktree,
		RemovedSession:   d.RemovedSession,
		Workspace:        d.Workspace,
		Task:             d.Task,
		Session:          d.Session,
		Queue:            d.Queue,
		Sends:            d.Sends,
	}
	if d.Worktree != nil {
		wt := withoutPorts(*d.Worktree)
		out.Worktree = &wt
	}
	keep := out.RemovedWorkspace != "" || out.RemovedWorktree != "" || out.RemovedSession != "" ||
		out.Workspace != nil || out.Task != nil || out.Worktree != nil || out.Session != nil || out.Queue != nil || out.Sends != nil
	return out, keep
}

func withoutPorts(wt domain.Worktree) domain.Worktree {
	wt.Ports = nil
	return wt
}

func orEmpty[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

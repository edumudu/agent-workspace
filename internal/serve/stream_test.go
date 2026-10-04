package serve_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/serve"
)

func wsURL(ts *httptest.Server) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/stream"
}

func dialStream(t *testing.T, ts *httptest.Server, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h := http.Header{}
	if origin != "" {
		h.Set("Origin", origin)
	}
	c, resp, err := websocket.Dial(ctx, wsURL(ts), &websocket.DialOptions{HTTPHeader: h})
	if c != nil {
		t.Cleanup(func() { _ = c.CloseNow() })
	}
	return c, resp, err
}

func openStream(t *testing.T, ts *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	c, _, err := dialStream(t, ts, publicURL)
	if err != nil {
		t.Fatal(err)
	}
	send(t, c, map[string]string{"token": token})
	return c
}

func send(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func next(t *testing.T, c *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return data
}

func closedWith(t *testing.T, c *websocket.Conn, within time.Duration) (websocket.StatusCode, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	for {
		_, data, err := c.Read(ctx)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			t.Fatalf("still open after %v (last frame %s)", within, data)
		}
		var ce websocket.CloseError
		if errors.As(err, &ce) {
			return ce.Code, ce.Reason
		}
		return -1, err.Error()
	}
}

func streamState() rpc.State {
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	return rpc.State{
		Seq:        41,
		Workspaces: []domain.Workspace{{Root: "/home/me/api", Kind: domain.WorkspaceSingle, LastUsed: at}},
		Tasks:      []domain.Task{{ID: "t1", Source: domain.TaskText, Text: "fix the login redirect"}},
		Worktrees: []domain.Worktree{{ID: "w1", Repo: "api", Path: "/home/me/.agentws/worktrees/api-login", Branch: "login",
			SessionID: "s1", Ports: []domain.Port{{Port: 5173, PID: 4242}}}},
		Sessions: []domain.Session{{ID: "s1", TaskID: "t1", Harness: domain.HarnessClaude, Model: "opus", State: domain.StateRunning,
			WorktreeIDs: []string{"w1"}}},
		Events:    []domain.SessionEvent{{SessionID: "s1", Kind: domain.EventUserPromptSubmit, At: at}},
		Subagents: []domain.Subagent{{SessionID: "s1", ID: "a1"}},
		Queue:     []domain.LaunchItem{{ID: "q1", Ref: "#42", Workspace: "/home/me/api"}},
		Drafts:    []domain.ReviewDraft{{Session: "s1"}},
		Sends:     []domain.QueuedSend{{ID: "m1", Session: "s1", Text: "and add a test", QueuedAt: at}},
	}
}

func TestServeStreamRefusesAnotherOrigin(t *testing.T) {
	f := newFakeDaemon()
	f.devices[goodToken] = phone
	_, ts := startServer(t, f, serve.Config{URL: publicURL})
	for _, origin := range []string{"https://evil.example", "http://agentws.example.ts.net", "https://agentws.example.ts.net:8443", ""} {
		_, resp, err := dialStream(t, ts, origin)
		if err == nil {
			t.Fatalf("origin %q: the stream opened", origin)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q: response %v", origin, resp)
		}
	}
	for _, origin := range []string{"https://AgentWS.example.ts.net", "https://agentws.example.ts.net:443"} {
		c, _, err := dialStream(t, ts, origin)
		if err != nil {
			t.Fatalf("origin %q refused: %v", origin, err)
		}
		_ = c.CloseNow()
	}
}

func TestServeStreamRefusesEveryOriginWithoutAPublicURL(t *testing.T) {
	f := newFakeDaemon()
	_, ts := startServer(t, f, serve.Config{})
	_, resp, err := dialStream(t, ts, publicURL)
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("err %v resp %v", err, resp)
	}
}

func TestServeStreamClosesWithoutATokenInTime(t *testing.T) {
	f := newFakeDaemon()
	f.devices[goodToken] = phone
	_, ts := startServer(t, f, serve.Config{URL: publicURL, AuthTimeout: 100 * time.Millisecond})
	c, _, err := dialStream(t, ts, publicURL)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	code, _ := closedWith(t, c, 2*time.Second)
	if code != serve.CloseUnauthorized {
		t.Fatalf("closed with %d", code)
	}
	if waited := time.Since(start); waited < 80*time.Millisecond {
		t.Fatalf("closed after %v, before the timeout", waited)
	}
}

func TestServeStreamClosesOnAWrongFirstFrame(t *testing.T) {
	for name, first := range map[string]any{
		"unknown token": map[string]string{"token": "wrong-token"},
		"no token":      map[string]string{"watch": "s1"},
		"not an object": []int{1},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeDaemon()
			f.devices[goodToken] = phone
			_, ts := startServer(t, f, serve.Config{URL: publicURL})
			c, _, err := dialStream(t, ts, publicURL)
			if err != nil {
				t.Fatal(err)
			}
			send(t, c, first)
			if code, reason := closedWith(t, c, 2*time.Second); code != serve.CloseUnauthorized {
				t.Fatalf("closed with %d %q", code, reason)
			}
			if f.subscribers() != 1 {
				t.Fatalf("an unauthenticated stream subscribed (%d subscribers)", f.subscribers())
			}
		})
	}
}

func TestServeStreamSendsTheFilteredStateThenItsDiffs(t *testing.T) {
	f := newFakeDaemon()
	f.devices[goodToken] = phone
	f.state = streamState()
	_, ts := startServer(t, f, serve.Config{URL: publicURL})
	c := openStream(t, ts, goodToken)
	golden(t, "stream-state.json", next(t, c))

	session := domain.Session{ID: "s1", TaskID: "t1", Harness: domain.HarnessClaude, State: domain.StatePermission}
	sends := []domain.QueuedSend{}
	f.broadcast(rpc.Diff{Seq: 42, Event: &domain.SessionEvent{SessionID: "s1", Kind: domain.EventPermissionRequest}, Session: &session})
	f.broadcast(rpc.Diff{Seq: 43, Subagent: &domain.Subagent{SessionID: "s1", ID: "a2"}})
	f.broadcast(rpc.Diff{Seq: 44, Draft: &domain.ReviewDraft{Session: "s1"}})
	f.broadcast(rpc.Diff{Seq: 45, Worktree: &domain.Worktree{ID: "w1", Repo: "api", Branch: "login", Ports: []domain.Port{{Port: 8080}}}})
	f.broadcast(rpc.Diff{Seq: 46, Sends: &sends})
	f.broadcast(rpc.Diff{Seq: 47, RemovedSession: "s2"})
	var frames []json.RawMessage
	for range 4 {
		frames = append(frames, next(t, c))
	}
	all, _ := json.Marshal(frames)
	golden(t, "stream-diffs.json", all)
}

func TestServeStreamClosesWhenItsDeviceIsRevoked(t *testing.T) {
	f := newFakeDaemon()
	f.devices[goodToken] = phone
	f.devices["tablet-token"] = tablet
	_, ts := startServer(t, f, serve.Config{URL: publicURL})
	phoneA := openStream(t, ts, goodToken)
	phoneB := openStream(t, ts, goodToken)
	other := openStream(t, ts, "tablet-token")
	for _, c := range []*websocket.Conn{phoneA, phoneB, other} {
		next(t, c)
	}
	f.broadcast(rpc.Diff{Seq: 50, RevokedDevice: phone.ID})
	for _, c := range []*websocket.Conn{phoneA, phoneB} {
		if code, reason := closedWith(t, c, time.Second); code != serve.CloseUnauthorized {
			t.Fatalf("closed with %d %q", code, reason)
		}
	}
	f.broadcast(rpc.Diff{Seq: 51, RemovedSession: "s9"})
	var frame serve.Frame
	if err := json.Unmarshal(next(t, other), &frame); err != nil || frame.Diff == nil || frame.Diff.RemovedSession != "s9" {
		t.Fatalf("the other device's stream got %+v (%v)", frame, err)
	}
	late := openStream(t, ts, goodToken)
	if code, _ := closedWith(t, late, time.Second); code != serve.CloseUnauthorized {
		t.Fatalf("a revoked device opened a stream (closed with %d)", code)
	}
}

func TestServeStreamClosesWhenTheDaemonGoesAway(t *testing.T) {
	f := newFakeDaemon()
	f.devices[goodToken] = phone
	_, ts := startServer(t, f, serve.Config{URL: publicURL})
	c := openStream(t, ts, goodToken)
	next(t, c)
	f.mu.Lock()
	for conn := range f.subs {
		for _, ch := range f.subs[conn] {
			close(ch)
		}
		delete(f.subs, conn)
	}
	f.mu.Unlock()
	if code, reason := closedWith(t, c, time.Second); code != websocket.StatusTryAgainLater {
		t.Fatalf("closed with %d %q", code, reason)
	}
}

func TestServeStreamAnswersAnUnknownFrameWithAnError(t *testing.T) {
	f := newFakeDaemon()
	f.devices[goodToken] = phone
	_, ts := startServer(t, f, serve.Config{URL: publicURL})
	c := openStream(t, ts, goodToken)
	next(t, c)
	send(t, c, map[string]string{"hello": "there"})
	var frame serve.Frame
	if err := json.Unmarshal(next(t, c), &frame); err != nil || frame.Error == nil || frame.Error.Code != rpc.CodeBadRequest {
		t.Fatalf("frame %+v (%v)", frame, err)
	}
}

func TestServeStreamReleasesItsDaemonConnectionWhenTheClientLeaves(t *testing.T) {
	f := newFakeDaemon()
	f.devices[goodToken] = phone
	_, ts := startServer(t, f, serve.Config{URL: publicURL})
	c := openStream(t, ts, goodToken)
	next(t, c)
	if f.subscribers() != 2 {
		t.Fatalf("subscribers %d with one stream open", f.subscribers())
	}
	_ = c.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(2 * time.Second)
	for f.subscribers() != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("subscribers %d after the client left", f.subscribers())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

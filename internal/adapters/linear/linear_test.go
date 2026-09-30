package linear_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/adapters/linear"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

type api struct {
	*httptest.Server
	requests atomic.Int32
	auth     atomic.Value
	variable atomic.Value
}

func newAPI(t *testing.T, status int, body string) *api {
	t.Helper()
	a := &api{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.requests.Add(1)
		a.auth.Store(r.Header.Get("Authorization"))
		var req struct {
			Variables map[string]string `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		a.variable.Store(req.Variables["id"])
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(a.Close)
	return a
}

func linearTask() domain.Task {
	return domain.Task{Source: domain.TaskLinear, Ref: "ENG-42"}
}

func TestNamingLinearTitleFromTheIssue(t *testing.T) {
	a := newAPI(t, 200, `{"data":{"issue":{"title":"  Fix the login redirect "}}}`)
	c := linear.Client{Token: "lin_key", Endpoint: a.URL}
	start := time.Now()
	title, err := c.Title(context.Background(), linearTask())
	if err != nil || title != "Fix the login redirect" {
		t.Fatalf("Title = %q, %v", title, err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("took %v, want under the 2 s budget", took)
	}
	if got := a.auth.Load(); got != "lin_key" {
		t.Errorf("Authorization = %v, want the raw token", got)
	}
	if got := a.variable.Load(); got != "ENG-42" {
		t.Errorf("issue id = %v, want ENG-42", got)
	}
}

func TestNamingLinearSkipsWhatItCannotAnswer(t *testing.T) {
	a := newAPI(t, 200, `{"data":{"issue":{"title":"x"}}}`)
	cases := []struct {
		name   string
		client linear.Client
		task   domain.Task
	}{
		{"no token", linear.Client{Endpoint: a.URL}, linearTask()},
		{"a text task", linear.Client{Token: "k", Endpoint: a.URL}, domain.Task{Source: domain.TaskText, Text: "x"}},
		{"a pr task", linear.Client{Token: "k", Endpoint: a.URL}, domain.Task{Source: domain.TaskPR, Ref: "api#1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, err := c.client.Title(context.Background(), c.task)
			if title != "" || err != nil {
				t.Errorf("Title = %q, %v; want nothing", title, err)
			}
		})
	}
	if n := a.requests.Load(); n != 0 {
		t.Errorf("%d requests sent, want none", n)
	}
}

func TestNamingLinearFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"http error", 401, `{"errors":[{"message":"unauthorized"}]}`},
		{"graphql error", 200, `{"errors":[{"message":"Entity not found"}]}`},
		{"no such issue", 200, `{"data":{"issue":null}}`},
		{"not json", 200, `oops`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := newAPI(t, c.status, c.body)
			title, err := linear.Client{Token: "k", Endpoint: a.URL}.Title(context.Background(), linearTask())
			if err == nil || title != "" {
				t.Errorf("Title = %q, %v; want an error", title, err)
			}
		})
	}
}

func TestNamingLinearGivesUpWhenTheContextEnds(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer slow.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (linear.Client{Token: "k", Endpoint: slow.URL}).Title(ctx, linearTask()); err == nil {
		t.Error("want an error once the context ends")
	}
}

func TestNamingLinearToken(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if tok, err := linear.LoadToken(write("ok.json", `{"token":" lin_abc \n"}`)); err != nil || tok != "lin_abc" {
		t.Errorf("LoadToken = %q, %v", tok, err)
	}
	if tok, err := linear.LoadToken(filepath.Join(dir, "missing.json")); err != nil || tok != "" {
		t.Errorf("a missing file should mean no token, got %q, %v", tok, err)
	}
	if _, err := linear.LoadToken(write("bad.json", `{`)); err == nil {
		t.Error("a malformed file should be an error")
	}
}

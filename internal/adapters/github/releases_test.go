package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func releaseServer(t *testing.T, body string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestReleaseCheckerReturnsNewestPublishedTag(t *testing.T) {
	var hits atomic.Int32
	srv := releaseServer(t, `[{"tag_name":"v0.1.0-alpha.3","draft":true},{"tag_name":"v0.1.0-alpha.2","prerelease":true}]`, &hits)
	c := ReleaseChecker{URL: srv.URL, CachePath: filepath.Join(t.TempDir(), "release.json")}
	got, err := c.Latest(context.Background())
	if err != nil || got != "v0.1.0-alpha.2" {
		t.Fatalf("Latest = %q, %v", got, err)
	}
}

func TestReleaseCheckerCachesWithinTTL(t *testing.T) {
	var hits atomic.Int32
	srv := releaseServer(t, `[{"tag_name":"v0.1.0-alpha.2"}]`, &hits)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := ReleaseChecker{URL: srv.URL, CachePath: filepath.Join(t.TempDir(), "release.json"), TTL: time.Hour, Now: func() time.Time { return now }}
	for range 2 {
		if got, err := c.Latest(context.Background()); err != nil || got != "v0.1.0-alpha.2" {
			t.Fatalf("Latest = %q, %v", got, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 (second call cached)", hits.Load())
	}
	now = now.Add(2 * time.Hour)
	if _, err := c.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2 (cache expired)", hits.Load())
	}
}

func TestReleaseCheckerFailsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := ReleaseChecker{URL: srv.URL, CachePath: filepath.Join(t.TempDir(), "release.json")}
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("want an error")
	}
}

func TestReleaseCheckerPicksTheHighestTagWhateverTheListOrder(t *testing.T) {
	var hits atomic.Int32
	srv := releaseServer(t, `[{"tag_name":"v0.1.0-alpha.2"},{"tag_name":"nightly"},{"tag_name":"v0.1.0-alpha.10"}]`, &hits)
	c := ReleaseChecker{URL: srv.URL, CachePath: filepath.Join(t.TempDir(), "release.json")}
	got, err := c.Latest(context.Background())
	if err != nil || got != "v0.1.0-alpha.10" {
		t.Fatalf("Latest = %q, %v", got, err)
	}
}

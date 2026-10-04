package webpush

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	wp "github.com/SherClockHolmes/webpush-go"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

const (
	Subscriber  = "https://github.com/giovaniif/agent-workspace"
	TTL         = 24 * time.Hour
	sendTimeout = 15 * time.Second
)

type keyPair struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

type Sender struct {
	path   string
	client *http.Client
	mu     sync.Mutex
	keys   *keyPair
}

func New(path string, client *http.Client) *Sender {
	if client == nil {
		client = &http.Client{Timeout: sendTimeout}
	}
	return &Sender{path: path, client: client}
}

func (s *Sender) PublicKey() (string, error) {
	k, err := s.load()
	if err != nil {
		return "", err
	}
	return k.PublicKey, nil
}

func (s *Sender) Send(ctx context.Context, sub domain.PushSubscription, msg domain.PushMessage) error {
	k, err := s.load()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	resp, err := wp.SendNotificationWithContext(ctx, payload, &wp.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     wp.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &wp.Options{
		HTTPClient:      s.client,
		Subscriber:      Subscriber,
		TTL:             int(TTL / time.Second),
		Urgency:         wp.UrgencyHigh,
		VAPIDPublicKey:  k.PublicKey,
		VAPIDPrivateKey: k.PrivateKey,
	})
	if err != nil {
		return fmt.Errorf("push to %s: %w", hostOf(sub.Endpoint), err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return fmt.Errorf("push to %s: %s: %w", hostOf(sub.Endpoint), resp.Status, app.ErrPushGone)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("push to %s: %s", hostOf(sub.Endpoint), resp.Status)
	}
	return nil
}

func (s *Sender) load() (keyPair, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys != nil {
		return *s.keys, nil
	}
	k, err := s.read()
	if errors.Is(err, os.ErrNotExist) {
		k, err = s.create()
	}
	if err != nil {
		return keyPair{}, err
	}
	s.keys = &k
	return k, nil
}

func (s *Sender) read() (keyPair, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return keyPair{}, err
	}
	var k keyPair
	if err := json.Unmarshal(b, &k); err != nil || k.PublicKey == "" || k.PrivateKey == "" {
		return keyPair{}, fmt.Errorf("%s holds no VAPID key pair; move it away to make a new one", s.path)
	}
	return k, nil
}

func (s *Sender) create() (keyPair, error) {
	private, public, err := wp.GenerateVAPIDKeys()
	if err != nil {
		return keyPair{}, err
	}
	k := keyPair{PublicKey: public, PrivateKey: private}
	b, err := json.Marshal(k)
	if err != nil {
		return keyPair{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return keyPair{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".vapid-*")
	if err != nil {
		return keyPair{}, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return keyPair{}, err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return keyPair{}, err
	}
	if err := tmp.Close(); err != nil {
		return keyPair{}, err
	}
	if err := os.Link(tmp.Name(), s.path); errors.Is(err, os.ErrExist) {
		return s.read()
	} else if err != nil {
		return keyPair{}, err
	}
	return k, nil
}

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Host
	}
	return "the push service"
}

package daemon_test

import (
	"context"
	"sync"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

type pushed struct {
	endpoint string
	msg      domain.PushMessage
}

type fakePushProvider struct {
	mu   sync.Mutex
	sent chan pushed
	errs map[string]error
}

func newFakePushProvider() *fakePushProvider {
	return &fakePushProvider{sent: make(chan pushed, 64), errs: map[string]error{}}
}

func (f *fakePushProvider) PublicKey() (string, error) { return "BFakeVapidPublicKey", nil }

func (f *fakePushProvider) Send(_ context.Context, sub domain.PushSubscription, msg domain.PushMessage) error {
	f.mu.Lock()
	err := f.errs[sub.Endpoint]
	f.mu.Unlock()
	f.sent <- pushed{endpoint: sub.Endpoint, msg: msg}
	return err
}

func (f *fakePushProvider) fail(endpoint string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[endpoint] = err
}

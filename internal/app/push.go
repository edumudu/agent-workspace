package app

import (
	"context"
	"errors"
	"sync"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

var ErrPushGone = errors.New("the push service no longer has this subscription")

type PushProvider interface {
	PublicKey() (string, error)
	Send(ctx context.Context, sub domain.PushSubscription, msg domain.PushMessage) error
}

func SendPush(ctx context.Context, p PushProvider, subs []domain.PushSubscription, msg domain.PushMessage) ([]string, error) {
	errs := make([]error, len(subs))
	var wg sync.WaitGroup
	for i, sub := range subs {
		wg.Go(func() { errs[i] = p.Send(ctx, sub, msg) })
	}
	wg.Wait()
	var gone []string
	var failed []error
	for i, err := range errs {
		switch {
		case errors.Is(err, ErrPushGone):
			gone = append(gone, subs[i].Endpoint)
		case err != nil:
			failed = append(failed, err)
		}
	}
	return gone, errors.Join(failed...)
}

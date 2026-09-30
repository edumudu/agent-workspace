package app

import (
	"context"
	"errors"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// TitleResolver looks up a work item's title at its source. It returns an
// empty title, and no error, for a task it does not handle. It calls out to
// the network or a CLI, so it runs on workers, never on the daemon loop.
type TitleResolver interface {
	Title(ctx context.Context, task domain.Task) (string, error)
}

// TitleResolvers asks each resolver in turn and takes the first title. An
// error only surfaces when no resolver gave a title.
type TitleResolvers []TitleResolver

func (rs TitleResolvers) Title(ctx context.Context, task domain.Task) (string, error) {
	var errs []error
	for _, r := range rs {
		title, err := r.Title(ctx, task)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if title != "" {
			return title, nil
		}
	}
	return "", errors.Join(errs...)
}

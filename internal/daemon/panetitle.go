package daemon

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

const titleEvery = 250 * time.Millisecond

type titleInput struct {
	session domain.Session
	cwd     string
}

func (d *Daemon) paneTitles(ctx context.Context) {
	if d.hs.host == nil {
		return
	}
	set := map[app.PaneID]string{}
	var seen uint64
	stale := true
	tick, stop := ticker(titleEvery)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		var inputs []titleInput
		var wts []domain.Worktree
		changed := false
		ok := d.query(func(s *state) {
			if !stale && s.seq == seen {
				return
			}
			seen, changed, stale = s.seq, true, false
			for _, x := range s.sessions {
				if x.Pane != "" {
					x.WorktreeIDs = slices.Clone(x.WorktreeIDs)
					inputs = append(inputs, titleInput{session: x, cwd: s.hints.cwd[x.ID]})
				}
			}
			wts = make([]domain.Worktree, 0, len(s.worktrees))
			for _, w := range s.worktrees {
				if w.PR != nil {
					pr := *w.PR
					w.PR = &pr
				}
				wts = append(wts, w)
			}
		})
		if !ok || !changed {
			continue
		}
		slices.SortFunc(wts, func(a, b domain.Worktree) int { return strings.Compare(a.ID, b.ID) })
		want := make(map[app.PaneID]string, len(inputs))
		for _, in := range inputs {
			want[app.PaneID(in.session.Pane)] = domain.AgentTitle(in.session, wts, in.cwd)
		}
		for pane, title := range want {
			if set[pane] == title {
				continue
			}
			if d.hs.host.SetTitle(ctx, pane, title) != nil {
				stale = true
				continue
			}
			set[pane] = title
		}
		for pane := range set {
			if _, live := want[pane]; !live {
				delete(set, pane)
			}
		}
	}
}

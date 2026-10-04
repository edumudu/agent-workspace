package domain

import (
	"fmt"
	"path"
	"time"
)

func StateSince(s Session, events []SessionEvent) (time.Time, bool) {
	replay := Session{Focused: s.Focused}
	var since time.Time
	for _, ev := range events {
		if ev.SessionID != s.ID {
			continue
		}
		next, _ := replay.Apply(HarnessEvent{Kind: ev.Kind})
		if next.State != replay.State {
			since = ev.At
		}
		replay = next
	}
	if since.IsZero() || replay.State != s.State {
		return time.Time{}, false
	}
	return since, true
}

func WorktreeLabel(worktrees []Worktree) string {
	if len(worktrees) == 0 {
		return ""
	}
	where := path.Base(worktrees[0].Repo)
	if b := worktrees[0].Branch; b != "" {
		where += "@" + cutRunes(b, maxBannerBranch)
	}
	if more := len(worktrees) - 1; more > 0 {
		where += fmt.Sprintf(" +%d", more)
	}
	return where
}

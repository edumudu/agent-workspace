package domain

import (
	"fmt"
	"path/filepath"
	"time"
)

// why: a worktree an agent just created from the default branch must not be
// taken as merged.
const CleanupGrace = 4 * time.Hour

type CleanupAction string

const (
	CleanupRemove        CleanupAction = "remove"
	CleanupBackupThenAsk CleanupAction = "backup_then_ask"
	CleanupKeep          CleanupAction = "keep"
)

type CleanupFacts struct {
	InDefault    bool
	OnDefault    bool
	Uncommitted  int
	Holders      []string
	SessionLive  bool
	LastActivity time.Time
	// why: a non-empty Unknown keeps the worktree.
	Unknown string
}

type CleanupDecision struct {
	Worktree     Worktree
	Action       CleanupAction
	Reason       string
	BackupBranch string
}

// why: anything in use, live or recent is kept before merge state is even
// looked at; a merged but dirty worktree, or a detached one with commits of
// its own, is backed up and left for the user.
func PlanCleanup(w Worktree, f CleanupFacts, now time.Time) CleanupDecision {
	d := CleanupDecision{Worktree: w, Action: CleanupKeep}
	prMerged := w.PR != nil && w.PR.State == PRMerged
	switch {
	case filepath.Clean(w.Path) == filepath.Clean(w.Repo):
		d.Reason = "the main checkout"
	case f.Unknown != "":
		d.Reason = "facts unavailable: " + f.Unknown
	case len(f.Holders) > 0:
		d.Reason = "in use by " + f.Holders[0]
	case f.SessionLive:
		d.Reason = "its session is live"
	case now.Sub(f.LastActivity) < CleanupGrace:
		d.Reason = fmt.Sprintf("active within the last %v", CleanupGrace)
	case f.OnDefault:
		d.Reason = "on the default branch"
	case w.Branch == "" && !f.InDefault:
		d.Action = CleanupBackupThenAsk
		d.Reason = "detached with commits not in the default branch"
		d.BackupBranch = BackupBranchName(w)
	case !prMerged && !f.InDefault:
		d.Reason = "not merged"
	case f.Uncommitted > 0:
		d.Action = CleanupBackupThenAsk
		d.Reason = fmt.Sprintf("%d uncommitted changes", f.Uncommitted)
	case prMerged:
		d.Action = CleanupRemove
		d.Reason = fmt.Sprintf("PR #%d merged", w.PR.Number)
	default:
		d.Action = CleanupRemove
		d.Reason = "merged into the default branch"
	}
	return d
}

func BackupBranchName(w Worktree) string {
	return "backup/wt-" + filepath.Base(w.Path)
}

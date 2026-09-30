package domain

// SizePending is a DiskRow's Size while the background worker has not
// measured the worktree yet.
const SizePending int64 = -1

// DiskRow is one worktree's line in the disk view: its measured size and
// what cleanup would do with it.
type DiskRow struct {
	WorktreeID string
	Size       int64
	Action     CleanupAction
	Reason     string
}

// Reclaimable sums the size of every worktree cleanup would remove or back
// up and remove. pending counts those whose size is not known yet; they are
// left out of the total.
func Reclaimable(rows []DiskRow) (total int64, pending int) {
	for _, r := range rows {
		if r.Action != CleanupRemove && r.Action != CleanupBackupThenAsk {
			continue
		}
		if r.Size == SizePending {
			pending++
			continue
		}
		total += r.Size
	}
	return total, pending
}

// TotalSize sums every measured row; pending counts the ones still being
// measured.
func TotalSize(rows []DiskRow) (total int64, pending int) {
	for _, r := range rows {
		if r.Size == SizePending {
			pending++
			continue
		}
		total += r.Size
	}
	return total, pending
}

// WorktreeStatus is the worktrees view's state column, as in the mockup: what
// the worktree is doing, from its owner session and cleanup's decision. owner
// is nil for a worktree no session owns.
func WorktreeStatus(w Worktree, owner *Session, a CleanupAction) string {
	switch {
	case a == CleanupBackupThenAsk && w.Branch == "":
		return "! detached"
	case a == CleanupBackupThenAsk:
		return "! merged, dirty"
	case a == CleanupRemove, w.PR != nil && w.PR.State == PRMerged:
		return "✓ merged"
	case owner == nil || owner.Ended:
		return "○ idle"
	}
	switch owner.State {
	case StateRunning:
		return "◐ in use"
	case StateWaiting, StatePermission:
		return "✳ in use"
	case StateDone:
		return "● in use"
	}
	return "○ idle"
}

package domain

const SizePending int64 = -1

type DiskRow struct {
	WorktreeID string
	Size       int64
	Action     CleanupAction
	Reason     string
}

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

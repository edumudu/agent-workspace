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

func DiskState(a CleanupAction) string {
	switch a {
	case CleanupRemove:
		return "merged"
	case CleanupBackupThenAsk:
		return "needs you"
	default:
		return "keep"
	}
}

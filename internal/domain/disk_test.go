package domain

import "testing"

func TestReclaimableSumsWorktreesTheEngineWouldRemoveOrBackUp(t *testing.T) {
	rows := []DiskRow{
		{WorktreeID: "a", Size: 100, Action: CleanupRemove},
		{WorktreeID: "b", Size: 40, Action: CleanupBackupThenAsk},
		{WorktreeID: "c", Size: 1000, Action: CleanupKeep},
		{WorktreeID: "d", Size: 7, Action: CleanupRemove},
	}
	total, pending := Reclaimable(rows)
	if total != 147 || pending != 0 {
		t.Errorf("Reclaimable = %d, %d pending; want 147, 0", total, pending)
	}
}

func TestReclaimableLeavesOutSizesNotComputedYetAndCountsThem(t *testing.T) {
	rows := []DiskRow{
		{WorktreeID: "a", Size: SizePending, Action: CleanupRemove},
		{WorktreeID: "b", Size: 40, Action: CleanupRemove},
		{WorktreeID: "c", Size: SizePending, Action: CleanupKeep},
	}
	total, pending := Reclaimable(rows)
	if total != 40 || pending != 1 {
		t.Errorf("Reclaimable = %d, %d pending; want 40, 1 (a kept row's pending size does not count)", total, pending)
	}
}

func TestDiskStateNamesWhatTheDecisionMeansForTheUser(t *testing.T) {
	cases := map[CleanupAction]string{
		CleanupRemove:        "merged",
		CleanupBackupThenAsk: "needs you",
		CleanupKeep:          "keep",
	}
	for action, want := range cases {
		if got := DiskState(action); got != want {
			t.Errorf("DiskState(%s) = %q, want %q", action, got, want)
		}
	}
}

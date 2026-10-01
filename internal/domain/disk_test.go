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

func TestTotalSizeSumsEveryMeasuredWorktreeAndCountsTheRest(t *testing.T) {
	rows := []DiskRow{
		{WorktreeID: "a", Size: 100, Action: CleanupKeep},
		{WorktreeID: "b", Size: SizePending, Action: CleanupRemove},
		{WorktreeID: "c", Size: 5, Action: CleanupRemove},
	}
	total, pending := TotalSize(rows)
	if total != 105 || pending != 1 {
		t.Errorf("TotalSize = %d, %d pending; want 105, 1", total, pending)
	}
}

func TestWorktreeStatusSaysWhatTheWorktreeIsDoing(t *testing.T) {
	merged := &PullRequest{Number: 1, State: PRMerged}
	cases := []struct {
		name   string
		w      Worktree
		owner  *Session
		action CleanupAction
		want   string
	}{
		{"its session is running", Worktree{Branch: "a"}, &Session{State: StateRunning}, CleanupKeep, "◐ in use"},
		{"its session waits on you", Worktree{Branch: "a"}, &Session{State: StateWaiting}, CleanupKeep, "✳ in use"},
		{"its session is done", Worktree{Branch: "a"}, &Session{State: StateDone}, CleanupKeep, "● in use"},
		{"its session is idle", Worktree{Branch: "a"}, &Session{State: StateIdle}, CleanupKeep, "○ idle"},
		{"its session ended", Worktree{Branch: "a"}, &Session{State: StateIdle, Ended: true}, CleanupKeep, "○ idle"},
		{"no session", Worktree{Branch: "a"}, nil, CleanupKeep, "○ idle"},
		{"cleanup would remove it", Worktree{Branch: "a"}, nil, CleanupRemove, "✓ merged"},
		{"merged but kept for now", Worktree{Branch: "a", PR: merged}, &Session{State: StateRunning}, CleanupKeep, "✓ merged"},
		{"merged with changes", Worktree{Branch: "a", PR: merged}, nil, CleanupBackupThenAsk, "! merged, dirty"},
		{"detached and not in the default branch", Worktree{}, nil, CleanupBackupThenAsk, "! detached"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WorktreeStatus(c.w, c.owner, c.action); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

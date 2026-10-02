package daemon

import (
	"os"
	"syscall"
	"testing"
)

// why: a child forked while the daemon runs shares the lock's open file
// until it execs, the way this dup does; a restart must not wait on it.
func TestReleaseFreesTheLockEvenWhileAForkedChildStillHoldsItsFile(t *testing.T) {
	home, err := os.MkdirTemp("/tmp", "agentws-l")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	lock, err := Acquire(home)
	if err != nil {
		t.Fatal(err)
	}
	child, err := syscall.Dup(int(lock.file.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Close(child) })
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	again, err := Acquire(home)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	_ = again.Release()
}

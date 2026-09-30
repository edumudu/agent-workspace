// Package procs implements app.ProcessTable with netstat, lsof and process
// signals. It only reads processes, and signals a group only when asked to.
package procs

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

var _ app.ProcessTable = Table{}

const (
	// DefaultGrace is how long Terminate waits after SIGTERM before SIGKILL.
	DefaultGrace = 3 * time.Second
	pollEvery    = 25 * time.Millisecond
	// killWait bounds the wait for the group to vanish after SIGKILL.
	killWait = time.Second
)

// Table reads listeners with netstat and lsof. Grace is DefaultGrace when zero.
type Table struct {
	Grace time.Duration
}

// Listeners reads every listening TCP socket from `netstat -anv -p tcp`, then
// asks lsof for the group, full command and cwd of just those processes.
// lsof alone would also list the sockets, but it walks every process's file
// descriptors and costs about 40 ms; netstat costs about 4 ms. A process
// lsof cannot read (another user's) gets no cwd, so it maps to no worktree.
func (Table) Listeners(ctx context.Context) ([]domain.Listener, error) {
	sockets, err := run(ctx, "netstat", "-anv", "-p", "tcp")
	if err != nil {
		return nil, err
	}
	listeners := parseNetstat(sockets)
	if len(listeners) == 0 {
		return nil, nil
	}
	found, err := run(ctx, "lsof", "-nP", "-a", "-d", "cwd", "-p", pidList(listeners), "+c", "0", "-Fpgcn")
	if err != nil {
		return nil, err
	}
	return merge(listeners, parseDetails(found)), nil
}

// run treats exit status 1 as success: lsof exits 1 when a listed pid
// vanished, and what it did print is still good.
func run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}

// Terminate signals the whole group, so a server's children go with it. It
// refuses group 1 and below and the caller's own group.
func (t Table) Terminate(ctx context.Context, pgid int) error {
	if pgid <= 1 || pgid == syscall.Getpgrp() {
		return fmt.Errorf("refusing to signal process group %d", pgid)
	}
	if err := signalGroup(pgid, syscall.SIGTERM); err != nil {
		return err
	}
	grace := t.Grace
	if grace == 0 {
		grace = DefaultGrace
	}
	if waitGone(ctx, pgid, grace) {
		return nil
	}
	if err := signalGroup(pgid, syscall.SIGKILL); err != nil {
		return err
	}
	if !waitGone(ctx, pgid, killWait) {
		return fmt.Errorf("process group %d survived SIGKILL", pgid)
	}
	return nil
}

func signalGroup(pgid int, sig syscall.Signal) error {
	err := syscall.Kill(-pgid, sig)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("signal process group %d: %w", pgid, err)
	}
	return nil
}

func waitGone(ctx context.Context, pgid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for {
		if errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH) {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(pollEvery)
	}
}

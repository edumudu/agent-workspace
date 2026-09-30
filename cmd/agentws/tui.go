package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/giovaniif/agent-workspace/internal/rpc"
	"github.com/giovaniif/agent-workspace/internal/tui"
)

func connect(ctx context.Context, home string) (*rpc.Client, error) {
	return rpc.Connect(ctx, rpc.SocketPath(home), func() error { return spawn(home) })
}

// attach opens the client layout, creating it on the first run, and replaces
// this process with the tmux client attached to it.
func attach(stderr io.Writer) int {
	home, err := rpc.Home()
	if err == nil {
		err = attachIn(home)
	}
	fmt.Fprintf(stderr, "agentws: %v\n", err)
	return 1
}

func attachIn(home string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	ctx := context.Background()
	c, err := connect(ctx, home)
	if err != nil {
		return err
	}
	opened, err := c.OpenClient(ctx, rpc.OpenClientParams{
		Command: []string{self, "tui"},
		Env:     map[string]string{"AGENTWS_HOME": home},
	})
	_ = c.Close()
	if err != nil {
		return err
	}
	bin, err := exec.LookPath(opened.Attach[0])
	if err != nil {
		return err
	}
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		// why: inside another tmux, attach refuses to nest unless TMUX is unset.
		if len(kv) < 5 || kv[:5] != "TMUX=" {
			env = append(env, kv)
		}
	}
	return syscall.Exec(bin, opened.Attach, env)
}

func runTUI(stderr io.Writer) int {
	home, err := rpc.Home()
	if err == nil {
		err = tuiIn(home)
	}
	if err != nil {
		fmt.Fprintf(stderr, "agentws tui: %v\n", err)
		return 1
	}
	return 0
}

func tuiIn(home string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	theme, err := tui.LoadTheme(filepath.Join(home, "config.toml"))
	if err != nil {
		return err
	}
	defaults, err := tui.LoadDefaults(filepath.Join(home, "config.toml"))
	if err != nil {
		return err
	}
	subscriber, err := connect(ctx, home)
	if err != nil {
		return err
	}
	defer func() { _ = subscriber.Close() }()
	caller, err := connect(ctx, home)
	if err != nil {
		return err
	}
	defer func() { _ = caller.Close() }()
	err = tui.Run(ctx, subscriber, caller, theme, defaults)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func runDebugSeed(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != "seed" {
		fmt.Fprintln(stderr, debugUsage)
		return 2
	}
	n, err := strconv.Atoi(args[1])
	if err != nil || n < 1 {
		fmt.Fprintln(stderr, debugUsage)
		return 2
	}
	home, err := rpc.Home()
	if err != nil {
		fmt.Fprintf(stderr, "agentws debug: %v\n", err)
		return 1
	}
	ctx := context.Background()
	c, err := connect(ctx, home)
	if err == nil {
		err = c.DebugSeed(ctx, n)
		_ = c.Close()
	}
	if err != nil {
		fmt.Fprintf(stderr, "agentws debug: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "seeded %d sessions\n", n)
	return 0
}

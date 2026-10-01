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

	"github.com/giovaniif/agent-workspace/internal/adapters/claude"
	"github.com/giovaniif/agent-workspace/internal/adapters/codex"
	"github.com/giovaniif/agent-workspace/internal/domain"
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
		Env:     map[string]string{"AGENTWS_HOME": home, "AGENTWS_LAUNCH_DIR": launchDir()},
		Dir:     launchDir(),
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

func runTUI(args []string, stderr io.Writer) int {
	newSession := len(args) == 1 && args[0] == "--new-session"
	if len(args) > 0 && !newSession {
		fmt.Fprintln(stderr, "usage: agentws tui [--new-session]")
		return 2
	}
	home, err := rpc.Home()
	if err == nil {
		err = tuiIn(home, newSession)
	}
	if err != nil {
		fmt.Fprintf(stderr, "agentws tui: %v\n", err)
		return 1
	}
	return 0
}

// tuiIn runs the sidebar, or with newSession the dialog alone, as the popup n
// opens.
func tuiIn(home string, newSession bool) error {
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
	fallback, err := tui.LoadFallback(filepath.Join(home, "config.toml"))
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
	opts := tui.Options{Theme: theme, Defaults: defaults, Fallback: fallback, NewSessionOnly: newSession}
	opts.HarnessDefaults = harnessDefaults()
	if newSession {
		// why: the daemon opens the popup where agentws was launched, so this is the folder to start in.
		opts.LaunchDir = launchDir()
	} else {
		// why: the inline dialog, used when the popup cannot open, starts in the same folder.
		opts.LaunchDir = os.Getenv("AGENTWS_LAUNCH_DIR")
	}
	if self, err := os.Executable(); err == nil && !newSession {
		opts.DialogPopup = rpc.ClientPopupParams{Command: []string{self, "tui", "--new-session"}, Env: map[string]string{"AGENTWS_HOME": home}}
	}
	err = tui.Run(ctx, subscriber, caller, opts)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func runDebugSeed(args []string, stdout, stderr io.Writer) int {
	codex := len(args) == 3 && args[2] == "--codex"
	if (len(args) != 2 && !codex) || args[0] != "seed" {
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
		err = c.DebugSeed(ctx, rpc.DebugSeedParams{Count: n, Codex: codex})
		_ = c.Close()
	}
	if err != nil {
		fmt.Fprintf(stderr, "agentws debug: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "seeded %d sessions\n", n)
	return 0
}

// harnessDefaults reads what Claude Code and Codex start with when a session
// is given no model or effort, so the dialog can name it. A file that cannot
// be read leaves that harness unnamed.
func harnessDefaults() map[domain.Harness]tui.Defaults {
	out := map[domain.Harness]tui.Defaults{}
	if path, err := claudeSettingsPath(os.Getenv); err == nil {
		if model, effort, err := claude.ReadDefaults(path); err == nil {
			byModel, _ := claude.ReadEffortByModel(path)
			out[domain.HarnessClaude] = tui.Defaults{Model: model, Effort: effort, EffortByModel: byModel}
		}
	}
	if dir, err := codexHome(os.Getenv); err == nil {
		if model, effort, err := codex.ReadDefaults(filepath.Join(dir, "config.toml")); err == nil {
			out[domain.HarnessCodex] = tui.Defaults{Model: model, Effort: effort}
		}
	}
	return out
}

// launchDir is the working directory, or "" when it cannot be read.
func launchDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

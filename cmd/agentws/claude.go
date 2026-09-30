package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/giovaniif/agent-workspace/internal/adapters/claude"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// claudeSettingsPath follows Claude Code: $CLAUDE_CONFIG_DIR, else ~/.claude.
func claudeSettingsPath(env func(string) string) (string, error) {
	dir := env("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "settings.json"), nil
}

func runSetupClaude(args []string, stdout, stderr io.Writer, env func(string) string, self string) int {
	fs := flag.NewFlagSet("setup claude", flag.ContinueOnError)
	fs.SetOutput(stderr)
	remove := fs.Bool("remove", false, "undo the setup")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	path, err := claudeSettingsPath(env)
	if err == nil {
		err = setupClaude(path, self, *remove, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "agentws setup claude: %v\n", err)
		return 1
	}
	return 0
}

func setupClaude(path, self string, remove bool, stdout io.Writer) error {
	if remove {
		if err := claude.Remove(path); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "removed agentws hooks and status line from %s\n", path)
		return nil
	}
	if err := claude.Setup(path, self); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "merged agentws hooks and status line into %s (backup: %s)\n", path, claude.BackupPath(path))
	return nil
}

// runStatusLine is Claude's status-line command after setup. The user's own
// command, if any, gets the same input and its output is printed unchanged;
// the report to the daemon runs alongside it and never delays it by more
// than hookTimeout. It always exits 0 so the status line keeps rendering.
func runStatusLine(args []string, stdin io.Reader, stdout io.Writer, home, pane string) int {
	fs := flag.NewFlagSet("statusline", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	_ = fs.String("harness", "claude", "the harness whose status line this is")
	chain := fs.String("chain", "", "the user's own status-line command")
	if err := fs.Parse(args); err != nil {
		logHook(home, err)
		return 0
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		logHook(home, err)
		return 0
	}
	sent := make(chan error, 1)
	go func() { sent <- reportStatus(home, pane, input) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := claude.Chain(ctx, *chain, input, stdout); err != nil {
		logHook(home, fmt.Errorf("statusline chain: %w", err))
	}
	if err := <-sent; err != nil {
		logHook(home, fmt.Errorf("statusline: %w", err))
	}
	return 0
}

func reportStatus(home, pane string, input []byte) error {
	if pane == "" {
		return errors.New("no TMUX_PANE")
	}
	report, err := claude.ParseStatus(input)
	if err != nil {
		return err
	}
	return sendOnce(home, rpc.MethodStatusLine, rpc.StatusLine{Pane: pane, Report: report})
}

// sendOnce writes one request and does not wait for the reply.
func sendOnce(home, method string, params any) error {
	nc, err := net.DialTimeout("unix", rpc.SocketPath(home), hookTimeout)
	if err != nil {
		return err
	}
	defer func() { _ = nc.Close() }()
	_ = nc.SetDeadline(time.Now().Add(hookTimeout))
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := json.Marshal(rpc.Request{V: rpc.Version, ID: 1, Method: method, Params: p})
	if err != nil {
		return err
	}
	_, err = nc.Write(append(req, '\n'))
	return err
}

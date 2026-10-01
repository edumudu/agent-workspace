package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	DefaultSocket = "agentws"
	sessionName   = "agentws"
	managedOption = "@agentws"
	placeholder   = "tail -f /dev/null"
	emptyState    = `printf 'No session in view.\n\nPress n to start one.\n'; exec tail -f /dev/null`

	// why: Claude Code and Codex leave ctrl+backslash unbound.
	FocusSidebarKey = `C-\`
)

// why: q/h keeps a # in the title from being read as a style or a format.
const titleFormat = "#{?@agentws_title, #{q/h:@agentws_title} ,}"

const configContents = `set -g status off
set -g prefix None
unbind-key -a
bind-key -n C-\\ select-pane -t :.0
set -g mouse off
set -g escape-time 0
set -g remain-on-exit off
set -g history-limit 50000
set -g default-terminal "tmux-256color"
set -g pane-border-status top
` + "set -g pane-border-format \"" + titleFormat + "\"\n" + `bind -n M-t if -F '#{m:agentws-popup-*,#{session_name}}' 'detach-client' 'send-keys M-t'
`

type Config struct {
	Socket     string
	ConfigPath string
}

type Host struct {
	socket     string
	configPath string

	configOnce sync.Once
	configErr  error
	// why: a server started with an older config lacks the title border options.
	titlesMu  sync.Mutex
	titlesOn  bool
	bufferSeq atomic.Uint64
}

func New(cfg Config) *Host {
	socket := cfg.Socket
	if socket == "" {
		socket = DefaultSocket
	}
	return &Host{socket: socket, configPath: cfg.ConfigPath}
}

func (h *Host) Close(ctx context.Context) error {
	_, err := h.run(ctx, "", "kill-server")
	if isServerGone(err) {
		return nil
	}
	return err
}

func (h *Host) ShowOption(ctx context.Context, name string) (string, error) {
	out, err := h.run(ctx, "", "show-options", "-gv", name)
	return strings.TrimSpace(out), err
}

func (h *Host) run(ctx context.Context, stdin string, args ...string) (string, error) {
	if err := h.ensureConfig(); err != nil {
		return "", err
	}
	full := append([]string{"-L", h.socket, "-f", h.configPath}, args...)
	cmd := exec.CommandContext(ctx, "tmux", full...)
	cmd.Env = withoutTmuxEnv(os.Environ())
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), &tmuxError{args: args, stderr: strings.TrimSpace(stderr.String()), err: err}
	}
	return stdout.String(), nil
}

func (h *Host) ensureConfig() error {
	h.configOnce.Do(func() {
		if h.configPath == "" {
			h.configErr = errors.New("tmux: Config.ConfigPath is required")
			return
		}
		if err := os.MkdirAll(filepath.Dir(h.configPath), 0o755); err != nil {
			h.configErr = err
			return
		}
		h.configErr = os.WriteFile(h.configPath, []byte(configContents), 0o644)
	})
	return h.configErr
}

func withoutTmuxEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "TMUX=") && !strings.HasPrefix(kv, "TMUX_PANE=") {
			out = append(out, kv)
		}
	}
	return out
}

type tmuxError struct {
	args   []string
	stderr string
	err    error
}

func (e *tmuxError) Error() string {
	return fmt.Sprintf("tmux %s: %s (%v)", strings.Join(e.args, " "), e.stderr, e.err)
}

func (e *tmuxError) Unwrap() error { return e.err }

func isServerGone(err error) bool {
	var te *tmuxError
	if !errors.As(err, &te) {
		return false
	}
	return strings.Contains(te.stderr, "no server running") ||
		strings.Contains(te.stderr, "error connecting") ||
		strings.Contains(te.stderr, "server exited")
}

func isMissingTarget(err error) bool {
	var te *tmuxError
	return errors.As(err, &te) && strings.Contains(te.stderr, "can't find")
}

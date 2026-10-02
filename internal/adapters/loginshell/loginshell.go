package loginshell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// why: a profile may print a greeting, an EXIT trap may print after the
// answer, and a line may be left unterminated, so PATH is read between two
// markers no profile prints.
const (
	start = "\x1eagentws-login-path\x1e"
	end   = "\x1fagentws-login-path\x1f"
)

func Path(ctx context.Context, shell string) (string, error) {
	if shell == "" {
		shell = "/bin/sh"
	}
	// why: no positional args, so fish (which has no $1) runs it too.
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", `printf '%s%s%s' '`+start+`' "$PATH" '`+end+`'`)
	// why: a background job the profile starts keeps stdout open; without a
	// delay Wait would block on it after the shell is killed.
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	path, ok := between(string(out))
	// why: ErrWaitDelay only means such a job still held stdout after the
	// shell answered and exited 0.
	if err != nil && (!errors.Is(err, exec.ErrWaitDelay) || !ok) {
		return "", fmt.Errorf("login shell %s: %w", shell, err)
	}
	if !ok {
		return "", errors.New("login shell " + shell + ": no PATH in its output")
	}
	return path, nil
}

func between(out string) (string, bool) {
	_, rest, ok := strings.Cut(out, start)
	if !ok {
		return "", false
	}
	path, _, ok := strings.Cut(rest, end)
	return path, ok
}

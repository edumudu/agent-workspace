package loginshell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// why: a profile may print a greeting or leave a line unterminated, so the
// PATH is read after a marker no profile prints.
const marker = "\x1eagentws-login-path\x1e"

func Path(ctx context.Context, shell string) (string, error) {
	if shell == "" {
		shell = "/bin/sh"
	}
	// why: no positional args, so fish (which has no $1) runs it too.
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", `printf '%s%s' '`+marker+`' "$PATH"`)
	// why: a background job the profile starts keeps stdout open; without a
	// delay Wait would block on it after the shell is killed.
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("login shell %s: %w", shell, err)
	}
	_, path, ok := strings.Cut(string(out), marker)
	if !ok {
		return "", errors.New("login shell " + shell + ": no PATH in its output")
	}
	return path, nil
}

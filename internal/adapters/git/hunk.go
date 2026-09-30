package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
)

var _ app.HunkGit = Review{}

func (Review) Stage(ctx context.Context, dir, patch string) error {
	return apply(ctx, dir, patch, "--cached")
}

// Revert checks the patch still applies in reverse, writes it to
// <git dir>/agentws/reverted/<unix nanos>.patch, and only then applies it.
func (Review) Revert(ctx context.Context, dir, patch string) error {
	if err := apply(ctx, dir, patch, "-R", "--check"); err != nil {
		return err
	}
	out, err := output(ctx, dir, "rev-parse", "--path-format=absolute", "--git-path", "agentws/reverted")
	if err != nil {
		return err
	}
	backups := strings.TrimSpace(string(out))
	if err := os.MkdirAll(backups, 0o700); err != nil {
		return err
	}
	name := filepath.Join(backups, strconv.FormatInt(time.Now().UnixNano(), 10)+".patch")
	if err := os.WriteFile(name, []byte(patch), 0o600); err != nil {
		return err
	}
	return apply(ctx, dir, patch, "-R")
}

// apply feeds patch to `git apply`, which changes nothing unless the whole
// patch applies.
func apply(ctx context.Context, dir, patch string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "apply", "--whitespace=nowarn"}, args...)...)
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git apply: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

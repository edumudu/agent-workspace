package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Adder creates worktrees with `git worktree add`.
type Adder struct{}

// AddWorktree makes branch at base and checks it out at path, creating
// path's parents. It fails if branch or path already exists.
func (Adder) AddWorktree(ctx context.Context, repo, path, branch, base string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "add", "-q", "-b", branch, path, base)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add %s: %w: %s", branch, err, strings.TrimSpace(string(out)))
	}
	return nil
}

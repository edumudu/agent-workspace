package git

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/giovaniif/agent-workspace/internal/app"
)

var _ app.ReviewGit = Review{}

// Review implements app.ReviewGit with the git CLI.
type Review struct{}

// WorkingTree stages everything into a throwaway copy of the index and
// writes it as a tree. Starting from the real index keeps git's stat cache,
// so only changed files are hashed.
func (Review) WorkingTree(ctx context.Context, dir string) (string, error) {
	gitDir, err := output(ctx, dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "agentws-index-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := copyInto(tmp, filepath.Join(strings.TrimSpace(string(gitDir)), "index")); err != nil {
		return "", err
	}
	env := []string{"GIT_INDEX_FILE=" + tmp.Name()}
	if _, err := outputEnv(ctx, dir, env, "add", "-A", "--", "."); err != nil {
		return "", err
	}
	tree, err := outputEnv(ctx, dir, env, "write-tree")
	return strings.TrimSpace(string(tree)), err
}

// copyInto fills dst with src, closes it and gives it src's mtime; a missing
// src (a repo with nothing staged yet) leaves dst empty, which git reads as
// an empty index.
func copyInto(dst *os.File, src string) error {
	in, err := os.Open(src)
	if errors.Is(err, os.ErrNotExist) {
		return dst.Close()
	}
	if err != nil {
		_ = dst.Close()
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		_ = dst.Close()
		return err
	}
	_, err = io.Copy(dst, in)
	if err = errors.Join(err, dst.Close()); err != nil {
		return err
	}
	// why: git re-hashes entries not older than the index file (its racy-git check); a fresh mtime would hide same-size edits made in the second of the last commit.
	return os.Chtimes(dst.Name(), info.ModTime(), info.ModTime())
}

func (Review) PointRef(ctx context.Context, dir, ref, tree string) error {
	// why: a snapshot commit needs an identity, and the user may have none configured.
	env := []string{
		"GIT_AUTHOR_NAME=agentws", "GIT_AUTHOR_EMAIL=agentws@localhost",
		"GIT_COMMITTER_NAME=agentws", "GIT_COMMITTER_EMAIL=agentws@localhost",
	}
	commit, err := outputEnv(ctx, dir, env, "commit-tree", tree, "-m", "agentws turn snapshot")
	if err != nil {
		return err
	}
	_, err = output(ctx, dir, "update-ref", ref, strings.TrimSpace(string(commit)))
	return err
}

func (Review) TurnRefs(ctx context.Context, dir string) ([]string, error) {
	out, err := output(ctx, dir, "for-each-ref", "--format=%(refname)", "refs/agentws/turns/")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

func (Review) DeleteRefs(ctx context.Context, dir string, refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	var stdin strings.Builder
	for _, r := range refs {
		stdin.WriteString("delete " + r + "\n")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "update-ref", "--stdin")
	cmd.Stdin = strings.NewReader(stdin.String())
	return cmd.Run()
}

func (Review) Resolve(ctx context.Context, dir, rev string) (string, error) {
	out, err := output(ctx, dir, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	return strings.TrimSpace(string(out)), err
}

func (Review) MergeBase(ctx context.Context, dir, rev string) (string, error) {
	out, err := output(ctx, dir, "merge-base", rev, "HEAD")
	return strings.TrimSpace(string(out)), err
}

// Diff pins every option a user's config could change about the output
// ParseDiff reads.
func (Review) Diff(ctx context.Context, dir, from, tree string) (string, error) {
	out, err := output(ctx, dir, "diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--find-renames", "--full-index", "--src-prefix=a/", "--dst-prefix=b/", "--no-relative", from, tree, "--")
	return string(out), err
}

func outputEnv(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0"), env...)
	return cmd.Output()
}

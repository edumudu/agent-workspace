package nvim

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Editor struct{}

func (Editor) Installed() bool {
	_, err := exec.LookPath("nvim")
	return err == nil
}

func (Editor) Eval(ctx context.Context, socket, expr string) error {
	cmd := exec.CommandContext(ctx, "nvim", "--server", socket, "--remote-expr", expr)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nvim --server %s: %s (%w)", socket, strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

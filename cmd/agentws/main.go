package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"

	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = ""
)

var stubs = []string{"cleanup"}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return attach(stderr)
	}
	switch cmd := args[0]; {
	case cmd == "tui":
		return runTUI(stderr)
	case cmd == "debug":
		return runDebug(args[1:], stdout, stderr)
	case cmd == "version":
		fmt.Fprintf(stdout, "agentws %s (commit %s)\n", version, buildCommit())
		return 0
	case cmd == "hook":
		home, _ := rpc.Home()
		return runHook(args[1:], os.Stdin, stdout, home, os.Getenv("TMUX_PANE"))
	case cmd == "setup":
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintf(stderr, "agentws setup: %v\n", err)
			return 1
		}
		return runSetup(args[1:], stdout, stderr, os.Getenv, self)
	case cmd == "statusline":
		home, _ := rpc.Home()
		return runStatusLine(args[1:], os.Stdin, stdout, home, os.Getenv("TMUX_PANE"))
	case cmd == "daemon":
		return runDaemon(args[1:], stdout, stderr)
	case cmd == "setup-worktree":
		return runSetupWorktree(args[1:], stdout, stderr)
	case cmd == "workspace":
		return runWorkspace(args[1:], stdout, stderr)
	case cmd == "worktree":
		return runWorktree(args[1:], stdout, stderr)

	case cmd == "new":
		return runNew(args[1:], stdout, stderr)
	case slices.Contains(stubs, cmd):
		fmt.Fprintf(stderr, "agentws %s: not implemented yet\n", cmd)
		return 1
	default:
		fmt.Fprintf(stderr, "agentws: unknown command %q\n", cmd)
		usage(stderr)
		return 2
	}
}

func buildCommit() string {
	if commit != "" {
		return commit
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				return s.Value
			}
		}
	}
	return "unknown"
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: agentws [daemon|workspace|worktree|setup|setup-worktree|tui|debug|hook|statusline|new|cleanup|version]")
}

package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = ""
)

var stubs = []string{"daemon", "tui", "hook", "new", "cleanup"}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch cmd := args[0]; {
	case cmd == "version":
		fmt.Fprintf(stdout, "agentws %s (commit %s)\n", version, buildCommit())
		return 0
	case isStub(cmd):
		fmt.Fprintf(stderr, "agentws %s: not implemented yet\n", cmd)
		return 1
	default:
		fmt.Fprintf(stderr, "agentws: unknown command %q\n", cmd)
		usage(stderr)
		return 2
	}
}

func isStub(cmd string) bool {
	for _, s := range stubs {
		if s == cmd {
			return true
		}
	}
	return false
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
	fmt.Fprintln(w, "usage: agentws <daemon|tui|hook|new|cleanup|version>")
}

// Package version identifies this build. Release and dev builds stamp it with
//
//	go build -ldflags "-X github.com/giovaniif/agent-workspace/internal/version.Version=v1.2.3 -X github.com/giovaniif/agent-workspace/internal/version.Commit=abc123"
//
// The daemon and its clients compare String on every request and refuse to
// talk across builds.
package version

import (
	"os"
	"sync"
	"time"
)

var (
	Version = "dev"
	Commit  = ""
)

func String() string {
	if Commit == "" {
		return Version
	}
	return Version + "+" + Commit
}

var builtAt = sync.OnceValue(func() time.Time {
	exe, err := os.Executable()
	if err != nil {
		return time.Time{}
	}
	info, err := os.Stat(exe)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
})

// BuiltAt is the modification time of the running executable, which tells
// which side of a mismatch is older. Zero when it cannot be read.
func BuiltAt() time.Time { return builtAt() }

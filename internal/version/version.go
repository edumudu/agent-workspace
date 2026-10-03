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

func BuiltAt() time.Time { return builtAt() }

package daemon

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/giovaniif/agent-workspace/internal/adapters/claude"
	"github.com/giovaniif/agent-workspace/internal/adapters/codex"
	wsfs "github.com/giovaniif/agent-workspace/internal/adapters/fs"
	gitadapter "github.com/giovaniif/agent-workspace/internal/adapters/git"
	"github.com/giovaniif/agent-workspace/internal/adapters/notify"
	"github.com/giovaniif/agent-workspace/internal/adapters/sqlite"
	"github.com/giovaniif/agent-workspace/internal/adapters/tmux"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

// Run is the whole daemon: it takes the lock in home, restores state from
// home/state.db, and serves rpc.SocketPath(home) until ctx is done.
func Run(ctx context.Context, home string) (err error) {
	lock, err := Acquire(home)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Release()) }()

	store, err := sqlite.Open(filepath.Join(home, "state.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()

	// why: tests and manual validation point this at a throwaway server so the real one is untouched.
	host := tmux.New(tmux.Config{
		Socket:     os.Getenv("AGENTWS_TMUX_SOCKET"),
		ConfigPath: filepath.Join(home, "tmux.conf"),
	})
	sounds, err := notify.LoadSounds(filepath.Join(home, "notify.json"))
	if err != nil {
		log.Printf("notify.json ignored: %v", err)
	}
	banners := notify.New()
	d, err := New(store, os.Getpid(),
		WithWorkspaces(wsfs.FS{}, gitadapter.Inspector{}),
		WithHarnesses(host, claude.Adapter{}, codex.Adapter{}),
		WithNotifier(banners, banners, sounds))
	if err != nil {
		return err
	}
	d.SetClientHost(host)

	sock := rpc.SocketPath(home)
	// why: holding the lock means any socket file left here belongs to a dead daemon.
	if err := os.Remove(sock); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(sock) }()
	if err := os.Chmod(sock, 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	return d.Serve(ctx, ln)
}

// Package rpc is the daemon protocol and its client. Every message is one JSON
// object per line on the Unix socket. See ARCHITECTURE.md, "RPC protocol".
package rpc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// Version is the protocol version every message carries as "v". The daemon
// rejects any other version with CodeUnsupportedVersion.
const Version = 1

const (
	MethodStatus          = "status"
	MethodSubscribe       = "subscribe"
	MethodHook            = "hook"
	MethodWorkspaceAdd    = "workspace.add"
	MethodWorkspaceList   = "workspace.list"
	MethodWorkspaceRemove = "workspace.remove"
)

// Hook is one harness hook event as `agentws hook` received it. Payload is
// the hook's stdin JSON, untouched.
type Hook struct {
	Harness string          `json:"harness"`
	Event   string          `json:"event"`
	Pane    string          `json:"pane"`
	At      time.Time       `json:"at"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// HookReply is the daemon's answer to a hook. Output, when set, is printed
// on the hook's stdout for the harness to read.
type HookReply struct {
	Output json.RawMessage `json:"output,omitempty"`
}

const (
	CodeUnsupportedVersion = "unsupported_version"
	CodeUnknownMethod      = "unknown_method"
	CodeBadRequest         = "bad_request"
	CodeNotFound           = "not_found"
)

// WorkspaceAddParams.Path must be absolute. Adding a known root again
// refreshes it and marks it last used.
type WorkspaceAddParams struct {
	Path string `json:"path"`
}

type WorkspaceRemoveParams struct {
	Root string `json:"root"`
}

// WorkspaceList is every registered workspace by root; LastUsed is the root
// of the most recently used one, empty when there are none.
type WorkspaceList struct {
	Workspaces []domain.Workspace `json:"workspaces"`
	LastUsed   string             `json:"last_used"`
}

type Request struct {
	V      int             `json:"v"`
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response answers the request with the same ID. A subscribe request gets one
// Response carrying the State as Result, then one per change carrying Diff.
type Response struct {
	V      int             `json:"v"`
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Diff   *Diff           `json:"diff,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

type Status struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Sessions  int       `json:"sessions"`
	Worktrees int       `json:"worktrees"`
}

// State is the full daemon state as of Seq; the diffs that follow it start
// at Seq+1.
type State struct {
	Seq        uint64             `json:"seq"`
	Workspaces []domain.Workspace `json:"workspaces"`
	Tasks      []domain.Task      `json:"tasks"`
	Worktrees  []domain.Worktree  `json:"worktrees"`
	Sessions   []domain.Session   `json:"sessions"`
}

// Diff is one change: exactly one field besides Seq is set. Every field but
// RemovedWorkspace replaces the entity with the same key; RemovedWorkspace is
// the root of a workspace to drop.
type Diff struct {
	Seq              uint64            `json:"seq"`
	RemovedWorkspace string            `json:"removed_workspace,omitempty"`
	Workspace        *domain.Workspace `json:"workspace,omitempty"`
	Task      *domain.Task      `json:"task,omitempty"`
	Worktree  *domain.Worktree  `json:"worktree,omitempty"`
	Session   *domain.Session   `json:"session,omitempty"`
}

// Home is $AGENTWS_HOME, or ~/.agentws.
func Home() (string, error) {
	if home := os.Getenv("AGENTWS_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agentws"), nil
}

func SocketPath(home string) string { return filepath.Join(home, "agentws.sock") }

package rpc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// why: the daemon rejects any other version with CodeUnsupportedVersion.
const Version = 1

const (
	MethodStatus          = "status"
	MethodSubscribe       = "subscribe"
	MethodHook            = "hook"
	MethodWorkspaceAdd    = "workspace.add"
	MethodWorkspaceList   = "workspace.list"
	MethodWorkspaceRemove = "workspace.remove"
	MethodOpenClient      = "client.open"
	MethodFocusMain       = "client.focus_main"
	MethodDebugSeed       = "debug.seed"
	MethodStatusLine      = "statusline"
	MethodLaunch          = "session.launch"
	MethodSessionMute     = "session.mute"
	// why: focus also clears the unread marker and blurs the session that was in
	// view; with a client layout open it swaps the session's pane into the main slot.
	MethodSessionFocus     = "session.focus"
	MethodWorktreeAssign   = "worktree.assign"
	MethodNewSession       = "session.new"
	MethodEndSession       = "session.end"
	MethodSessionRename    = "session.rename"
	MethodSessionUnpin     = "session.unpin"
	MethodLauncherEnqueue  = "launcher.enqueue"
	MethodLauncherDrop     = "launcher.drop"
	MethodLauncherRetarget = "launcher.retarget"
	MethodPortsKill        = "ports.kill"
	MethodReviewOpen       = "review.open"
	MethodReviewViewed     = "review.viewed"
	MethodReviewSend       = "review.send"
	MethodReviewHunk       = "review.hunk"
	MethodClientReview     = "client.review"
	MethodClientPopup      = "client.popup"
	MethodClientDetach     = "client.detach"
	MethodCleanupPlan      = "cleanup.plan"
	MethodCleanupRun       = "cleanup.run"
	// why: disk.view never waits for du: sizes not measured yet are domain.SizePending.
	MethodDiskView        = "disk.view"
	MethodCleanupWorktree = "cleanup.worktree"
)

type DiskView struct {
	Free  uint64 `json:"free"`
	Total uint64 `json:"total"`
	// why: 0 means automatic cleanup is off.
	AutoCleanEvery time.Duration    `json:"auto_clean_every"`
	DepsStore      *DepsStore       `json:"deps_store,omitempty"`
	Rows           []domain.DiskRow `json:"rows"`
	Recent         []RecentCleanup  `json:"recent"`
}

type DepsStore struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type RecentCleanup struct {
	At      time.Time            `json:"at"`
	Path    string               `json:"path"`
	Branch  string               `json:"branch,omitempty"`
	Action  domain.CleanupAction `json:"action"`
	Outcome string               `json:"outcome"`
}

// why: with Backup, a dirty or detached worktree is backed up and then removed;
// without it, only one cleanup would remove anyway goes.
type CleanupWorktreeParams struct {
	Path   string `json:"path"`
	Backup bool   `json:"backup,omitempty"`
}

type ReviewParams struct {
	Session  string             `json:"session"`
	Scope    domain.ReviewScope `json:"scope"`
	Worktree string             `json:"worktree,omitempty"`
}

// why: marks may be stale; domain.IsViewed tells which still apply.
type Review struct {
	Scope     domain.ReviewScope      `json:"scope"`
	Worktrees []domain.WorktreeReview `json:"worktrees"`
	Viewed    []domain.ViewedMark     `json:"viewed"`
	// why: the draft has no ID before the first comment.
	Draft domain.ReviewDraft `json:"draft"`
}

type ReviewSendParams struct {
	Session string `json:"session"`
}

type HunkParams struct {
	Session  string            `json:"session"`
	Worktree string            `json:"worktree"`
	File     domain.FileDiff   `json:"file"`
	Hunk     int               `json:"hunk"`
	Action   domain.HunkAction `json:"action"`
}

type ViewedParams struct {
	Mark   domain.ViewedMark `json:"mark"`
	Viewed bool              `json:"viewed"`
}

type ClientReviewParams struct {
	Open bool `json:"open"`
}

type ClientPopupParams struct {
	Command []string          `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
}

// why: the daemon kills only groups that serve a port it lists on some worktree.
type PortsKillParams struct {
	PGIDs []int `json:"pgids"`
}

type PortsKilled struct {
	Killed []int `json:"killed"`
}

// why: Outcome is empty in a plan.
type CleanupItem struct {
	Path    string               `json:"path"`
	Branch  string               `json:"branch,omitempty"`
	Action  domain.CleanupAction `json:"action"`
	Reason  string               `json:"reason"`
	Outcome string               `json:"outcome,omitempty"`
}

type SessionMuteParams struct {
	ID    string `json:"id"`
	Muted bool   `json:"muted"`
}

// why: the name goes on the task, so every session on that task shares it.
type SessionRenameParams struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SessionFocusParams struct {
	ID string `json:"id"`
}

// why: an empty Workspace means the last used one.
type NewSessionParams struct {
	Workspace string `json:"workspace,omitempty"`
	WorkItem  string `json:"work_item"`
	Harness   string `json:"harness"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

// why: Input is split on whitespace; an empty Workspace means the last used one.
type LauncherEnqueueParams struct {
	Workspace string `json:"workspace,omitempty"`
	Input     string `json:"input"`
	Harness   string `json:"harness"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

// why: an issue already queued or running is in neither list.
type LauncherEnqueued struct {
	Queued   []string `json:"queued"`
	Rejected []string `json:"rejected"`
}

type LauncherItemRef struct {
	ID string `json:"id"`
}

type LauncherRetargetParams struct {
	ID      string `json:"id"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

type SessionRef struct {
	ID string `json:"id"`
}

type StatusLine struct {
	Pane   string              `json:"pane"`
	Report domain.StatusReport `json:"report"`
}

type LaunchParams struct {
	Harness string `json:"harness"`
	Name    string `json:"name,omitempty"`
	Dir     string `json:"dir"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
}

type Hook struct {
	Harness string          `json:"harness"`
	Event   string          `json:"event"`
	Pane    string          `json:"pane"`
	At      time.Time       `json:"at"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type HookReply struct {
	Output json.RawMessage `json:"output,omitempty"`
}

const (
	CodeUnsupportedVersion = "unsupported_version"
	CodeUnknownMethod      = "unknown_method"
	CodeBadRequest         = "bad_request"
	CodeNotFound           = "not_found"
	CodeUnavailable        = "unavailable"
	CodeFailed             = "failed"
	CodeLaunchFailed       = "launch_failed"
	// CodeVersionMismatch: the client and the daemon are different builds.
	// The message says which side to restart.
	CodeVersionMismatch = "version_mismatch"
)

// AnyBuild reports whether method is answered across builds: status and
// stop must reach a stale daemon, and hooks may come from another install's
// binary that shares this AGENTWS_HOME.
func AnyBuild(method string) bool {
	return method == MethodStatus || method == MethodHook || method == MethodStatusLine
}

// Mismatch explains a build mismatch, naming the older side as the one to
// restart. Built times are Unix seconds; zero means unknown.
func Mismatch(daemonBuild, clientBuild string, daemonBuilt, clientBuilt int64) *Error {
	fix := "restart the daemon: run `agentws daemon stop`; the next command starts the new one"
	if clientBuilt != 0 && daemonBuilt > clientBuilt {
		fix = "restart this client: it is older than the daemon"
	}
	if daemonBuild == "" {
		daemonBuild = "an unknown build"
	}
	return &Error{Code: CodeVersionMismatch, Message: fmt.Sprintf("daemon runs agentws %s but this client is %s; %s", daemonBuild, clientBuild, fix)}
}

// why: Path must be absolute. Adding a known root again refreshes it and marks
// it last used.
type WorkspaceAddParams struct {
	Path string `json:"path"`
}

type WorkspaceRemoveParams struct {
	Root string `json:"root"`
}

type WorkspaceList struct {
	Workspaces []domain.Workspace `json:"workspaces"`
	LastUsed   string             `json:"last_used"`
}

type OpenClientParams struct {
	Command []string          `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
	Dir     string            `json:"dir,omitempty"`
}

type OpenClient struct {
	Slot   string   `json:"slot"`
	Attach []string `json:"attach"`
}

type DebugSeedParams struct {
	Count int  `json:"count"`
	Codex bool `json:"codex,omitempty"`
}

// why: an empty Session unassigns the worktree.
type WorktreeAssignParams struct {
	ID      string `json:"id"`
	Session string `json:"session"`
}

type Request struct {
	V      int             `json:"v"`
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	// Build is the client's version.String and BuiltAt its executable's
	// Unix mtime. Raw hook lines leave both out.
	Build   string `json:"build,omitempty"`
	BuiltAt int64  `json:"built_at,omitempty"`
}

// why: a subscribe request gets one Response carrying the State as Result, then
// one per change carrying Diff.
type Response struct {
	V      int             `json:"v"`
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Diff   *Diff           `json:"diff,omitempty"`
	Error  *Error          `json:"error,omitempty"`
	Build  string          `json:"build,omitempty"`
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

// why: the diffs that follow a State start at Seq+1.
type State struct {
	Seq        uint64                `json:"seq"`
	Workspaces []domain.Workspace    `json:"workspaces"`
	Tasks      []domain.Task         `json:"tasks"`
	Worktrees  []domain.Worktree     `json:"worktrees"`
	Sessions   []domain.Session      `json:"sessions"`
	Events     []domain.SessionEvent `json:"events"`
	Subagents  []domain.Subagent     `json:"subagents"`
	Queue      []domain.LaunchItem   `json:"queue"`
	Drafts     []domain.ReviewDraft  `json:"drafts"`
}

// why: Diff is one change: exactly one field besides Seq is set, except that a hook
// sets Session and Event together. Every field but the Removed ones and Event
// replaces the entity with the same key; RemovedWorkspace is the root of a
// workspace to drop, RemovedWorktree the ID of a worktree, RemovedSession the
// ID of a session, and Event is
// appended to its session's events. A subagent change is a diff of its own,
// replacing the subagent with the same session and ID. Queue replaces the
// whole launcher queue.
type Diff struct {
	Seq              uint64               `json:"seq"`
	RemovedWorkspace string               `json:"removed_workspace,omitempty"`
	RemovedWorktree  string               `json:"removed_worktree,omitempty"`
	RemovedSession   string               `json:"removed_session,omitempty"`
	Workspace        *domain.Workspace    `json:"workspace,omitempty"`
	Task             *domain.Task         `json:"task,omitempty"`
	Worktree         *domain.Worktree     `json:"worktree,omitempty"`
	Session          *domain.Session      `json:"session,omitempty"`
	Event            *domain.SessionEvent `json:"event,omitempty"`
	Subagent         *domain.Subagent     `json:"subagent,omitempty"`
	Queue            *[]domain.LaunchItem `json:"queue,omitempty"`
	// why: a sent draft no longer counts. Comment is set with it when a comment was
	// just added.
	Draft   *domain.ReviewDraft   `json:"draft,omitempty"`
	Comment *domain.ReviewComment `json:"comment,omitempty"`
}

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

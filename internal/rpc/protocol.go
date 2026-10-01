// Package rpc is the daemon protocol and its client. Every message is one JSON
// object per line on the Unix socket. See ARCHITECTURE.md, "RPC protocol".
package rpc

import (
	"encoding/json"
	"fmt"
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
	// MethodOpenClient returns the client layout, creating it if it is gone.
	MethodOpenClient = "client.open"
	MethodFocusMain  = "client.focus_main"
	// MethodDebugSeed adds fake tasks, sessions and worktrees for manual testing.
	MethodDebugSeed  = "debug.seed"
	MethodStatusLine = "statusline"
	MethodLaunch     = "session.launch"
	// MethodSessionMute sets whether a session's banners are silenced.
	MethodSessionMute = "session.mute"
	// MethodSessionFocus marks the session as the one in view, clearing its
	// unread marker and blurring the session that was in view. With a client
	// layout open it also swaps the session's pane into the main slot.
	MethodSessionFocus   = "session.focus"
	MethodWorktreeAssign = "worktree.assign"
	MethodNewSession     = "session.new"
	MethodEndSession     = "session.end"
	// MethodSessionRename pins a name on the session's task; MethodSessionUnpin
	// takes it off, so the name is automatic again.
	MethodSessionRename = "session.rename"
	MethodSessionUnpin  = "session.unpin"
	// MethodLauncherEnqueue queues the Linear issues in a pasted text;
	// MethodLauncherDrop takes a waiting one off the queue and
	// MethodLauncherRetarget changes the harness, model and effort it starts with.
	MethodLauncherEnqueue  = "launcher.enqueue"
	MethodLauncherDrop     = "launcher.drop"
	MethodLauncherRetarget = "launcher.retarget"
	// MethodPortsKill terminates the process groups behind worktree ports.
	MethodPortsKill    = "ports.kill"
	MethodReviewOpen   = "review.open"
	MethodReviewViewed = "review.viewed"
	// MethodReviewSend sends the draft as one prompt once the agent is
	// between tools.
	MethodReviewSend = "review.send"
	// MethodReviewHunk stages or reverts one hunk of a file in a worktree the
	// session owns.
	MethodReviewHunk = "review.hunk"
	// MethodClientReview widens the client's sidebar pane for the review, or
	// puts it back.
	MethodClientReview = "client.review"
	// MethodClientPopup runs a command in a centred popup over the attached
	// client; the popup closes when the command exits.
	MethodClientPopup = "client.popup"
	// MethodClientDetach detaches the terminals attached to the client layout,
	// which keeps running.
	MethodClientDetach = "client.detach"
	// MethodCleanupPlan returns the cleanup plan without acting on it;
	// MethodCleanupRun executes it. Both answer []CleanupItem.
	MethodCleanupPlan = "cleanup.plan"
	MethodCleanupRun  = "cleanup.run"
	// MethodDiskView answers a DiskView. It never waits for du: sizes not
	// measured yet are domain.SizePending.
	MethodDiskView = "disk.view"
	// MethodCleanupWorktree removes one worktree through the cleanup engine
	// and answers its CleanupItem.
	MethodCleanupWorktree = "cleanup.worktree"
)

// DiskView is the worktrees and disk view. Rows are in worktree order;
// domain.Reclaimable folds them into the reclaimable total.
type DiskView struct {
	Free  uint64 `json:"free"`
	Total uint64 `json:"total"`
	// AutoCleanEvery is 0 when automatic cleanup is off.
	AutoCleanEvery time.Duration `json:"auto_clean_every"`
	// DepsStore is nil when no shared dependency store is configured.
	DepsStore *DepsStore       `json:"deps_store,omitempty"`
	Rows      []domain.DiskRow `json:"rows"`
	Recent    []RecentCleanup  `json:"recent"`
}

type DepsStore struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// RecentCleanup is one line of the cleanup audit log, newest first in a DiskView.
type RecentCleanup struct {
	At      time.Time            `json:"at"`
	Path    string               `json:"path"`
	Branch  string               `json:"branch,omitempty"`
	Action  domain.CleanupAction `json:"action"`
	Outcome string               `json:"outcome"`
}

// CleanupWorktreeParams names a worktree by path. With Backup, a dirty or
// detached one is backed up and then removed; without it, only one cleanup
// would remove anyway goes.
type CleanupWorktreeParams struct {
	Path   string `json:"path"`
	Backup bool   `json:"backup,omitempty"`
}

// ReviewParams asks for Session's review in Scope, of one worktree ID or,
// when Worktree is empty, of all its worktrees.
type ReviewParams struct {
	Session  string             `json:"session"`
	Scope    domain.ReviewScope `json:"scope"`
	Worktree string             `json:"worktree,omitempty"`
}

// Review holds each worktree's files and every viewed mark in those
// worktrees; domain.IsViewed tells which marks still apply.
type Review struct {
	Scope     domain.ReviewScope      `json:"scope"`
	Worktrees []domain.WorktreeReview `json:"worktrees"`
	Viewed    []domain.ViewedMark     `json:"viewed"`
	// Draft is the session's unsent draft; it has no ID before the first
	// comment.
	Draft domain.ReviewDraft `json:"draft"`
}

type ReviewSendParams struct {
	Session string `json:"session"`
}

// HunkParams names hunk Hunk of File, as the review showed it, in the
// worktree whose ID is Worktree.
type HunkParams struct {
	Session  string            `json:"session"`
	Worktree string            `json:"worktree"`
	File     domain.FileDiff   `json:"file"`
	Hunk     int               `json:"hunk"`
	Action   domain.HunkAction `json:"action"`
}

// ViewedParams marks Mark viewed, or clears the mark for its worktree and
// path when Viewed is false.
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

// PortsKillParams names process groups by PGID, as a domain.Port carries it.
// The daemon kills only groups that serve a port it lists on some worktree.
type PortsKillParams struct {
	PGIDs []int `json:"pgids"`
}

// PortsKilled is the groups that are gone.
type PortsKilled struct {
	Killed []int `json:"killed"`
}

// CleanupItem is one worktree's cleanup decision. Outcome is empty in a plan.
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

// SessionRenameParams pins Name on the task the session belongs to, which
// every session on that task shares.
type SessionRenameParams struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SessionFocusParams struct {
	ID string `json:"id"`
}

// NewSessionParams starts Harness on WorkItem in the workspace rooted at
// Workspace, or the last used one when it is empty. The result is the new
// domain.Session.
type NewSessionParams struct {
	Workspace string `json:"workspace,omitempty"`
	WorkItem  string `json:"work_item"`
	Harness   string `json:"harness"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

// LauncherEnqueueParams queues every Linear issue URL found in Input, split
// on whitespace, to start with Harness, Model and Effort in the workspace
// rooted at Workspace (the last used one when empty).
type LauncherEnqueueParams struct {
	Workspace string `json:"workspace,omitempty"`
	Input     string `json:"input"`
	Harness   string `json:"harness"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

// LauncherEnqueued lists the refs that joined the queue and the input words
// that were not Linear issue URLs. An issue already queued or running is in
// neither.
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

// session.end answers a SessionRef with the ended domain.Session.
type SessionRef struct {
	ID string `json:"id"`
}

// StatusLine is one harness status-line update from the session on Pane.
type StatusLine struct {
	Pane   string              `json:"pane"`
	Report domain.StatusReport `json:"report"`
}

// LaunchParams starts Harness in a new pane in Dir. The result is the new
// domain.Session.
type LaunchParams struct {
	Harness string `json:"harness"`
	Name    string `json:"name,omitempty"`
	Dir     string `json:"dir"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
}

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

// OpenClientParams is the TUI command the layout's left pane runs.
type OpenClientParams struct {
	Command []string          `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
	// Dir is where agentws was launched; the new-session popup opens there.
	Dir string `json:"dir,omitempty"`
}

// OpenClient names the layout's window (Slot) and the argv that attaches the
// caller's terminal to it.
type OpenClient struct {
	Slot   string   `json:"slot"`
	Attach []string `json:"attach"`
}

// DebugSeedParams: Codex also seeds Codex sessions and limits, for trying the
// UI of a harness that is not set up.
type DebugSeedParams struct {
	Count int  `json:"count"`
	Codex bool `json:"codex,omitempty"`
}

// WorktreeAssignParams gives worktree ID to session Session; an empty
// Session unassigns it.
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

// Response answers the request with the same ID. A subscribe request gets one
// Response carrying the State as Result, then one per change carrying Diff.
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

// State is the full daemon state as of Seq; the diffs that follow it start
// at Seq+1.
type State struct {
	Seq        uint64             `json:"seq"`
	Workspaces []domain.Workspace `json:"workspaces"`
	Tasks      []domain.Task      `json:"tasks"`
	Worktrees  []domain.Worktree  `json:"worktrees"`
	Sessions   []domain.Session   `json:"sessions"`
	// Events are each session's newest hook events, oldest first.
	Events []domain.SessionEvent `json:"events"`
	// Subagents are every session's tracked subagents, in the order they started.
	Subagents []domain.Subagent `json:"subagents"`
	// Queue is the launcher's issues that have not become sessions yet.
	Queue []domain.LaunchItem `json:"queue"`
	// Drafts are each session's open or queued review draft, by ID.
	Drafts []domain.ReviewDraft `json:"drafts"`
}

// Diff is one change: exactly one field besides Seq is set, except that a hook
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
	// Draft replaces its session's draft; one that is sent no longer
	// counts. Comment is set with it when a comment was just added.
	Draft   *domain.ReviewDraft   `json:"draft,omitempty"`
	Comment *domain.ReviewComment `json:"comment,omitempty"`
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

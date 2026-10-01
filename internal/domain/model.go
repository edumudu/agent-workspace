package domain

import "time"

type WorkspaceKind string

const (
	WorkspaceSingle        WorkspaceKind = "single"
	WorkspaceOrchestration WorkspaceKind = "orchestration"
)

type Workspace struct {
	Root     string
	Kind     WorkspaceKind
	Repos    []Repo
	LastUsed time.Time
}

// Repo carries the facts a background refresh fills in; they are empty until
// the first refresh, or when git cannot answer.
type Repo struct {
	Name          string
	Path          string
	DefaultBranch string
	Branch        string
	ChangedFiles  int
}

type TaskSource string

const (
	TaskLinear TaskSource = "linear"
	TaskPR     TaskSource = "pr"
	TaskText   TaskSource = "text"
)

// Task is the work item sessions are grouped under. IssueTitle is the Linear
// issue title when Source is linear, and PRTitle the PR title when Source is
// pr; PinnedName is set when the user pins a name. URL is the Linear or PR
// link the task came from.
type Task struct {
	ID         string
	Source     TaskSource
	Ref        string
	Text       string
	IssueTitle string
	PRTitle    string
	PinnedName string
	URL        string
}

type PRState string

const (
	PROpen   PRState = "OPEN"
	PRMerged PRState = "MERGED"
	PRClosed PRState = "CLOSED"
)

// CheckState is a PR's check rollup. CheckNone also stands for a single
// check that neither passed nor failed, such as a skipped one.
type CheckState string

const (
	CheckNone    CheckState = ""
	CheckPending CheckState = "pending"
	CheckPassing CheckState = "passing"
	CheckFailing CheckState = "failing"
)

type PullRequest struct {
	Number int
	Title  string
	URL    string
	Head   string
	State  PRState
	Checks CheckState

	ReviewDecision    ReviewDecision
	Mergeable         Mergeable
	UnresolvedThreads int
	// BotComments counts bot comments since the last push.
	BotComments int
	Failing     []FailingCheck
}

type Harness string

const (
	HarnessClaude Harness = "claude"
	HarnessCodex  Harness = "codex"
)

type Worktree struct {
	ID          string
	Repo        string
	Path        string
	Branch      string
	PR          *PullRequest
	SubtaskSlug string
	// SessionID is empty when unassigned.
	SessionID string
	// Ports are the dev servers listening from inside the worktree. They are
	// live process state: the daemon does not persist them.
	Ports []Port
}

// Usage is what the harness last reported. LimitUsedPercent is the fullest
// of the session's Limits, the one that blocks it first.
type Usage struct {
	ContextLeftPercent int
	// HasContext is false until a report carried the context figure, so a new
	// session does not read as 0% left.
	HasContext       bool
	LimitUsedPercent int
}

// RateLimit is one usage window, such as five_hour, seven_day, or a
// per-model seven_day window. ResetsAt is Unix seconds, 0 when unknown.
type RateLimit struct {
	Window      string
	UsedPercent int
	ResetsAt    int64
}

// StatusReport is one status-line update. Empty fields and HasContext false
// mean the harness does not know the value yet.
type StatusReport struct {
	Model       string
	Effort      string
	ContextLeft int
	HasContext  bool
	Limits      []RateLimit
	// At is when the harness reported, used to stamp Limits.
	At time.Time
}

func (s Session) Report(r StatusReport) Session {
	s = s.confirmSwitches(r)
	if r.Model != "" {
		s.Model = r.Model
	}
	if r.Effort != "" {
		s.Effort = r.Effort
	}
	if r.HasContext {
		s.Usage.ContextLeftPercent, s.Usage.HasContext = r.ContextLeft, true
	}
	if len(r.Limits) > 0 {
		s.Limits = r.Limits
		s.LimitsAt = r.At
		s.Usage.LimitUsedPercent = 0
		for _, l := range r.Limits {
			s.Usage.LimitUsedPercent = max(s.Usage.LimitUsedPercent, l.UsedPercent)
		}
	}
	return s
}

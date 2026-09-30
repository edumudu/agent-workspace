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
// issue title when Source is linear; PinnedName is set when the user pins a name.
type Task struct {
	ID         string
	Source     TaskSource
	Ref        string
	Text       string
	IssueTitle string
	PinnedName string
}

type PullRequest struct {
	Number int
	Title  string
	URL    string
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
}

// Usage is what the harness last reported. LimitUsedPercent is the fullest
// of the session's Limits, the one that blocks it first.
type Usage struct {
	ContextLeftPercent int
	LimitUsedPercent   int
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
	if r.Model != "" {
		s.Model = r.Model
	}
	if r.Effort != "" {
		s.Effort = r.Effort
	}
	if r.HasContext {
		s.Usage.ContextLeftPercent = r.ContextLeft
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

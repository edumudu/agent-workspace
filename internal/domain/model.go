package domain

type WorkspaceKind string

const (
	WorkspaceSingle        WorkspaceKind = "single"
	WorkspaceOrchestration WorkspaceKind = "orchestration"
)

type Workspace struct {
	Root  string
	Kind  WorkspaceKind
	Repos []Repo
}

type Repo struct {
	Name string
	Path string
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

type Usage struct {
	ContextLeftPercent int
	LimitUsedPercent   int
}

package rpc

const (
	MethodShellToggle   = "shell.toggle"
	MethodShellFocus    = "shell.focus"
	MethodNvimToggle    = "nvim.toggle"
	MethodNvimOpen      = "nvim.open"
	MethodReviewComment = "review.comment"
)

// why: without a worktree the session's first one is used, else the directory its
// hooks last reported. Without a session, Worktree is required and its owner,
// if any, is the session.
type ShellParams struct {
	Session  string `json:"session"`
	Worktree string `json:"worktree,omitempty"`
	Popup    bool   `json:"popup,omitempty"`
}

type ShellResult struct {
	Pane  string `json:"pane"`
	Dir   string `json:"dir"`
	Shown bool   `json:"shown"`
}

// why: for nvim.open, Path is relative to the worktree, or absolute inside it,
// and Line starts at 1.
type NvimParams struct {
	Session  string `json:"session"`
	Worktree string `json:"worktree,omitempty"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
}

type NvimResult struct {
	Pane   string `json:"pane"`
	Socket string `json:"socket"`
	Shown  bool   `json:"shown"`
}

// why: EndLine zero means StartLine. File is absolute and the daemon places it
// in one of the session's worktrees; otherwise Worktree and Path name the file.
type CommentParams struct {
	Session   string `json:"session"`
	File      string `json:"file,omitempty"`
	Worktree  string `json:"worktree,omitempty"`
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line,omitempty"`
	Code      string `json:"code,omitempty"`
	Body      string `json:"body"`
	// why: Removed lines are old-side numbers: every one was deleted.
	Removed bool `json:"removed,omitempty"`
}

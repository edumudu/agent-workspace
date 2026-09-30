package rpc

const (
	// MethodShellToggle shows the session's shell below its agent pane, or
	// hides it again; with Popup it opens a popup instead.
	MethodShellToggle = "shell.toggle"
	// MethodNvimToggle swaps the session's nvim into the main slot, or the
	// agent pane back.
	MethodNvimToggle = "nvim.toggle"
	// MethodNvimOpen opens Path at Line in the session's nvim and shows it.
	MethodNvimOpen = "nvim.open"
	// MethodReviewComment adds a draft review comment to a session.
	MethodReviewComment = "review.comment"
)

// ShellParams names the session and, optionally, the worktree of its shell.
// Without a worktree the session's first one is used, else the directory its
// hooks last reported. Without a session, Worktree is required and its owner,
// if any, is the session.
type ShellParams struct {
	Session  string `json:"session"`
	Worktree string `json:"worktree,omitempty"`
	Popup    bool   `json:"popup,omitempty"`
}

// ShellResult is the shell's pane and directory; Shown is false when the
// call hid it.
type ShellResult struct {
	Pane  string `json:"pane"`
	Dir   string `json:"dir"`
	Shown bool   `json:"shown"`
}

// NvimParams picks a session's nvim. For nvim.open, Path is relative to the
// worktree, or absolute inside it, and Line starts at 1.
type NvimParams struct {
	Session  string `json:"session"`
	Worktree string `json:"worktree,omitempty"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// NvimResult is the nvim pane and the socket it listens on; Shown is false
// when the call put the agent pane back.
type NvimResult struct {
	Pane   string `json:"pane"`
	Socket string `json:"socket"`
	Shown  bool   `json:"shown"`
}

// CommentParams adds a draft comment on lines StartLine to EndLine (zero
// means StartLine) of a file. The file is either File, absolute, which the
// daemon places in one of the session's worktrees, or Worktree and Path.
// Code is the text of those lines as the commenter saw them.
type CommentParams struct {
	Session   string `json:"session"`
	File      string `json:"file,omitempty"`
	Worktree  string `json:"worktree,omitempty"`
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line,omitempty"`
	Code      string `json:"code,omitempty"`
	Body      string `json:"body"`
	// Removed says the lines are old-side numbers: every one was deleted.
	Removed bool `json:"removed,omitempty"`
}

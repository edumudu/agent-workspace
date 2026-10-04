package rpc

const (
	MethodShellToggle   = "shell.toggle"
	MethodShellFocus    = "shell.focus"
	MethodNvimToggle    = "nvim.toggle"
	MethodNvimOpen      = "nvim.open"
	MethodReviewComment = "review.comment"
)

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

type CommentParams struct {
	Session   string `json:"session"`
	File      string `json:"file,omitempty"`
	Worktree  string `json:"worktree,omitempty"`
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line,omitempty"`
	Code      string `json:"code,omitempty"`
	Body      string `json:"body"`
	Removed   bool   `json:"removed,omitempty"`
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const (
	reviewUsage  = "usage: agentws review <comment|scope> ..."
	commentUsage = "usage: agentws review comment [--session id] (--file abs-path | --worktree id --path rel-path) --start line [--end line] [--code text] --body text"
	scopeUsage   = "usage: agentws review scope [--session id] [--scope last_turn|uncommitted|branch] [--worktree id]"
)

var errBadComment = errors.New("bad comment arguments")

// sessionFromEnv is the default for --session: the shell and nvim panes the
// daemon starts carry their session in AGENTWS_SESSION.
func sessionFromEnv() string { return os.Getenv("AGENTWS_SESSION") }

func parseCommentArgs(args []string, stderr io.Writer) (rpc.CommentParams, error) {
	fs := flag.NewFlagSet("review comment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var p rpc.CommentParams
	fs.StringVar(&p.Session, "session", sessionFromEnv(), "session id (default $AGENTWS_SESSION)")
	fs.StringVar(&p.File, "file", "", "absolute path of the file")
	fs.StringVar(&p.Worktree, "worktree", "", "worktree id, with --path")
	fs.StringVar(&p.Path, "path", "", "path in the worktree, with --worktree")
	fs.IntVar(&p.StartLine, "start", 0, "first line")
	fs.IntVar(&p.EndLine, "end", 0, "last line (default: the first)")
	fs.StringVar(&p.Code, "code", "", "the text of the commented lines")
	fs.StringVar(&p.Body, "body", "", "the comment")
	if err := fs.Parse(args); err != nil {
		return p, err
	}
	hasFile := p.File != "" || (p.Worktree != "" && p.Path != "")
	if p.Session == "" || !hasFile || p.StartLine < 1 || p.Body == "" {
		return p, errBadComment
	}
	return p, nil
}

func runReview(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, reviewUsage)
		return 2
	}
	switch args[0] {
	case "comment":
		p, err := parseCommentArgs(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, commentUsage)
			return 2
		}
		return callReview(stderr, "comment", func(ctx context.Context, c *rpc.Client) error {
			var out domain.DraftComment
			if err := c.Call(ctx, rpc.MethodReviewComment, p, &out); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "comment %s on %s:%d\n", out.ID, out.Path, out.StartLine)
			return nil
		})
	case "scope":
		p, err := parseScopeArgs(args[1:], stderr)
		if err != nil {
			fmt.Fprintln(stderr, scopeUsage)
			return 2
		}
		return callReview(stderr, "scope", func(ctx context.Context, c *rpc.Client) error {
			rev, err := c.Review(ctx, p)
			if err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(scopeRows(rev))
		})
	default:
		fmt.Fprintln(stderr, reviewUsage)
		return 2
	}
}

func callReview(stderr io.Writer, what string, f func(context.Context, *rpc.Client) error) int {
	ctx := context.Background()
	c, err := connectHome(ctx)
	if err == nil {
		err = f(ctx, c)
		_ = c.Close()
	}
	if err != nil {
		fmt.Fprintf(stderr, "agentws review %s: %v\n", what, err)
		return 1
	}
	return 0
}

func parseScopeArgs(args []string, stderr io.Writer) (rpc.ReviewParams, error) {
	fs := flag.NewFlagSet("review scope", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var p rpc.ReviewParams
	scope := fs.String("scope", string(domain.ScopeUncommitted), "last_turn, uncommitted or branch")
	fs.StringVar(&p.Session, "session", sessionFromEnv(), "session id (default $AGENTWS_SESSION)")
	fs.StringVar(&p.Worktree, "worktree", "", "one worktree id (default: all)")
	if err := fs.Parse(args); err != nil {
		return p, err
	}
	p.Scope = domain.ReviewScope(*scope)
	if p.Session == "" {
		return p, errBadComment
	}
	return p, nil
}

type scopeRow struct {
	Worktree string   `json:"worktree"`
	Path     string   `json:"path"`
	From     string   `json:"from"`
	Files    []string `json:"files"`
	Error    string   `json:"error,omitempty"`
}

func scopeRows(r rpc.Review) []scopeRow {
	rows := make([]scopeRow, 0, len(r.Worktrees))
	for _, w := range r.Worktrees {
		row := scopeRow{Worktree: w.Worktree.ID, Path: w.Worktree.Path, From: w.From, Files: []string{}, Error: w.Err}
		for _, f := range w.Files {
			row.Files = append(row.Files, f.Path)
		}
		rows = append(rows, row)
	}
	return rows
}

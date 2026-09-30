package domain

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ShellTarget is the shell a session gets for a directory. Key is unique per
// session and worktree, so the same shell is found again.
type ShellTarget struct {
	Key string
	Dir string
}

// ChooseShell picks the directory of a session's shell: the wanted worktree,
// else the session's first worktree by ID, else cwd. A wanted worktree the
// session does not own is refused rather than replaced by another.
func ChooseShell(session string, worktrees []Worktree, wanted, cwd string) (ShellTarget, bool) {
	var owned []Worktree
	for _, w := range worktrees {
		if w.SessionID == session {
			owned = append(owned, w)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].ID < owned[j].ID })
	for _, w := range owned {
		if wanted == "" || w.ID == wanted {
			return ShellTarget{Key: session + "/" + w.ID, Dir: w.Path}, true
		}
	}
	if wanted != "" || cwd == "" {
		return ShellTarget{}, false
	}
	return ShellTarget{Key: session + "/root", Dir: cwd}, true
}

// NvimOpenExpr is the expression a running nvim evaluates to edit path at
// line. The path goes in as a quoted string through fnameescape, so no
// character in it is read as a command.
func NvimOpenExpr(path string, line int) string {
	line = max(line, 1)
	return fmt.Sprintf("execute('edit +%d ' . fnameescape('%s'))", line, strings.ReplaceAll(path, "'", "''"))
}

// ResolveCommentFile finds which of the session's worktrees holds the
// absolute file, the innermost when they nest, and the file's path in it.
func ResolveCommentFile(session string, worktrees []Worktree, file string) (Worktree, string, bool) {
	if !filepath.IsAbs(file) {
		return Worktree{}, "", false
	}
	file = filepath.Clean(file)
	var best Worktree
	var rel string
	for _, w := range worktrees {
		if w.SessionID != session {
			continue
		}
		r, err := filepath.Rel(filepath.Clean(w.Path), file)
		if err != nil || r == "." || r == ".." || strings.HasPrefix(r, "../") {
			continue
		}
		if best.ID == "" || len(w.Path) > len(best.Path) {
			best, rel = w, r
		}
	}
	return best, rel, best.ID != ""
}

// NormalizeLines orders a comment's first and last line. An end of zero
// means the comment is on one line.
func NormalizeLines(start, end int) (int, int, bool) {
	if end == 0 {
		end = start
	}
	if start < 1 || end < 1 {
		return 0, 0, false
	}
	return min(start, end), max(start, end), true
}

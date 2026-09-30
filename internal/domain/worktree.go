package domain

import (
	"path/filepath"
	"strings"
	"time"
)

// ClaimWindow is how long after a `git worktree add` hook a new worktree is
// still credited to the session that ran it.
const ClaimWindow = 30 * time.Second

// Branch is empty when the worktree is detached.
type ListedWorktree struct {
	Path   string
	Branch string
}

// Main is the main checkout's path and is not among Worktrees.
type RepoListing struct {
	Main      string
	Worktrees []ListedWorktree
}

// SessionHint.Cwd is the cwd from the session's latest hook.
type SessionHint struct {
	ID  string
	Cwd string
}

// WorktreeClaim.Cwd is where the command ran; it ties the claim to a repo.
type WorktreeClaim struct {
	SessionID string
	Cwd       string
	Command   string
	At        time.Time
}

// IsWorktreeAdd matches any pipeline or list segment, and skips git's global
// options such as -C.
func IsWorktreeAdd(command string) bool {
	segments := strings.FieldsFunc(command, func(r rune) bool {
		return r == ';' || r == '&' || r == '|' || r == '\n'
	})
	for _, seg := range segments {
		if segmentIsWorktreeAdd(strings.Fields(seg)) {
			return true
		}
	}
	return false
}

func segmentIsWorktreeAdd(words []string) bool {
	if len(words) == 0 || filepath.Base(words[0]) != "git" {
		return false
	}
	rest := words[1:]
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		if rest[0] == "-C" || rest[0] == "-c" {
			rest = rest[1:]
		}
		if len(rest) > 0 {
			rest = rest[1:]
		}
	}
	return len(rest) >= 2 && rest[0] == "worktree" && rest[1] == "add"
}

// SubagentParent returns the checkout a Claude subagent worktree
// (<parent>/.claude/worktrees/agent-*) belongs to.
func SubagentParent(path string) (string, bool) {
	clean := filepath.Clean(path)
	if !strings.HasPrefix(filepath.Base(clean), "agent-") {
		return "", false
	}
	worktrees := filepath.Dir(clean)
	claude := filepath.Dir(worktrees)
	if filepath.Base(worktrees) != "worktrees" || filepath.Base(claude) != ".claude" {
		return "", false
	}
	return filepath.Dir(claude), true
}

// AttributeWorktree picks the session a newly seen worktree belongs to, or
// "" for unassigned. In order: the parent session of a subagent worktree, a
// session working inside it, then a recent `git worktree add` claim, which
// must name the path or branch when several sessions made one.
func AttributeWorktree(wt ListedWorktree, hints []SessionHint, claims []WorktreeClaim, now time.Time) string {
	if parent, ok := SubagentParent(wt.Path); ok {
		for _, h := range hints {
			if filepath.Clean(h.Cwd) == parent {
				return h.ID
			}
		}
	}
	for _, h := range hints {
		if h.Cwd != "" && within(h.Cwd, wt.Path) {
			return h.ID
		}
	}
	var recent []WorktreeClaim
	for _, c := range claims {
		if now.Sub(c.At) <= ClaimWindow {
			recent = append(recent, c)
		}
	}
	if id, ok := onlySession(recent); ok {
		return id
	}
	var naming []WorktreeClaim
	for _, c := range recent {
		if strings.Contains(c.Command, filepath.Base(wt.Path)) || (wt.Branch != "" && strings.Contains(c.Command, wt.Branch)) {
			naming = append(naming, c)
		}
	}
	id, _ := onlySession(naming)
	return id
}

// namedBy is the one session whose claims name the worktree by a whole
// word: its branch, or a path ending in its dir name.
func namedBy(path, branch string, claims []WorktreeClaim) string {
	base := filepath.Base(path)
	var naming []WorktreeClaim
	for _, c := range claims {
		for _, word := range strings.Fields(c.Command) {
			if word == branch || word == base || strings.HasSuffix(filepath.Clean(word), "/"+base) {
				naming = append(naming, c)
				break
			}
		}
	}
	id, _ := onlySession(naming)
	return id
}

// ReclaimWorktrees attaches unassigned worktrees that a recent claim names:
// a scan that runs while `git worktree add` does sees the worktree before
// the PostToolUse hook that claims it. Pre-existing worktrees stay
// unassigned unless a claim names them word for word. It returns only the
// ones it changed.
func ReclaimWorktrees(known []Worktree, claims []WorktreeClaim, now time.Time) []Worktree {
	var recent []WorktreeClaim
	for _, c := range claims {
		if now.Sub(c.At) <= ClaimWindow {
			recent = append(recent, c)
		}
	}
	if len(recent) == 0 {
		return nil
	}
	repoOf := func(cwd string) string {
		repo, depth := "", -1
		for _, w := range known {
			for _, dir := range []string{w.Path, w.Repo} {
				if cwd != "" && within(cwd, dir) && len(dir) > depth {
					repo, depth = w.Repo, len(dir)
				}
			}
		}
		return repo
	}
	var changed []Worktree
	for _, w := range known {
		if w.SessionID != "" || filepath.Clean(w.Path) == filepath.Clean(w.Repo) {
			continue
		}
		var sameRepo []WorktreeClaim
		for _, c := range recent {
			if repoOf(c.Cwd) == w.Repo {
				sameRepo = append(sameRepo, c)
			}
		}
		if id := namedBy(w.Path, w.Branch, sameRepo); id != "" {
			w.SessionID = id
			changed = append(changed, w)
		}
	}
	return changed
}

func onlySession(claims []WorktreeClaim) (string, bool) {
	if len(claims) == 0 {
		return "", false
	}
	for _, c := range claims[1:] {
		if c.SessionID != claims[0].SessionID {
			return "", false
		}
	}
	return claims[0].SessionID, true
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// ReconcileWorktrees compares the known worktrees of listing's repo with what
// git listed. changed holds new worktrees, owned by attribute, and known ones
// whose branch moved; removed holds the IDs git no longer lists.
func ReconcileWorktrees(known []Worktree, listing RepoListing, attribute func(ListedWorktree) string) (changed []Worktree, removed []string) {
	byID := map[string]Worktree{}
	for _, w := range known {
		if w.Repo == listing.Main {
			byID[w.ID] = w
		}
	}
	seen := map[string]bool{}
	for _, l := range listing.Worktrees {
		seen[l.Path] = true
		w, ok := byID[l.Path]
		if !ok {
			changed = append(changed, Worktree{ID: l.Path, Repo: listing.Main, Path: l.Path, Branch: l.Branch, SessionID: attribute(l)})
			continue
		}
		if w.Branch != l.Branch {
			w.Branch = l.Branch
			changed = append(changed, w)
		}
	}
	for _, w := range known {
		if w.Repo == listing.Main && !seen[w.ID] {
			removed = append(removed, w.ID)
		}
	}
	return changed, removed
}

// RollupChecks folds a PR's checks: any failure fails, then any pending is
// pending, then any pass passes.
func RollupChecks(states []CheckState) CheckState {
	out := CheckNone
	for _, s := range states {
		switch {
		case s == CheckFailing:
			return CheckFailing
		case s == CheckPending:
			out = CheckPending
		case s == CheckPassing && out == CheckNone:
			out = CheckPassing
		}
	}
	return out
}

// PRForBranch is the PR whose head is branch: the open one if any, else the
// most recent.
func PRForBranch(prs []PullRequest, branch string) *PullRequest {
	var best *PullRequest
	for i := range prs {
		p := &prs[i]
		if branch == "" || p.Head != branch {
			continue
		}
		switch {
		case best == nil:
			best = p
		case (p.State == PROpen) != (best.State == PROpen):
			if p.State == PROpen {
				best = p
			}
		case p.Number > best.Number:
			best = p
		}
	}
	if best == nil {
		return nil
	}
	pr := *best
	return &pr
}

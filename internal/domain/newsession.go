package domain

import (
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	linearKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+$`)
	prNumber  = regexp.MustCompile(`^[0-9]+$`)
	notSlug   = regexp.MustCompile(`[^a-z0-9]+`)
)

const (
	slugWords = 5
	slugMax   = 40
)

// ParseWorkItem reads what the user typed as the work item: a Linear issue
// URL, a GitHub PR URL, or anything else as free text. A URL that does not
// fit either shape is kept as text, never rejected.
func ParseWorkItem(input string) Task {
	input = strings.TrimSpace(input)
	text := Task{Source: TaskText, Text: input}
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return text
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch strings.TrimPrefix(u.Host, "www.") {
	case "linear.app":
		if len(parts) < 3 || parts[1] != "issue" || !linearKey.MatchString(parts[2]) {
			return text
		}
		t := Task{Source: TaskLinear, Ref: strings.ToUpper(parts[2]), URL: input}
		if len(parts) > 3 {
			t.IssueTitle = strings.ReplaceAll(parts[3], "-", " ")
		}
		return t
	case "github.com":
		if len(parts) < 4 || parts[2] != "pull" || !prNumber.MatchString(parts[3]) {
			return text
		}
		return Task{Source: TaskPR, Ref: parts[1] + "#" + parts[3], URL: input}
	}
	return text
}

// TaskSlug names a task's worktree dir and branch: the Linear key, the PR
// ref, or the first words of the text, lowercased with dashes.
func TaskSlug(t Task) string {
	src := t.Text
	if t.Source != TaskText {
		src = t.Ref
	}
	words := strings.FieldsFunc(notSlug.ReplaceAllString(strings.ToLower(src), " "), func(r rune) bool { return r == ' ' })
	if len(words) > slugWords {
		words = words[:slugWords]
	}
	slug := strings.Join(words, "-")
	if len(slug) > slugMax {
		slug = strings.TrimRight(slug[:slugMax], "-")
	}
	if slug == "" {
		return "task"
	}
	return slug
}

// FindTask is the known task for the same work item, so sessions started on
// it group together.
func FindTask(known []Task, t Task) (Task, bool) {
	for _, k := range known {
		if k.Source != t.Source {
			continue
		}
		if t.Source == TaskText {
			if t.Text != "" && k.Text == t.Text {
				return k, true
			}
		} else if k.Ref == t.Ref {
			return k, true
		}
	}
	return Task{}, false
}

// SessionPlan is where a new session starts. Worktree is set only for a
// single-repo workspace, and Dir is then the worktree's path.
type SessionPlan struct {
	Dir      string
	Worktree *WorktreePlan
}

// WorktreePlan is one `git worktree add` in RepoPath: a new Branch at Path,
// started from Base.
type WorktreePlan struct {
	Repo     string
	RepoPath string
	Path     string
	Branch   string
	Base     string
}

// PlanSessionStart places a session. An orchestration root starts at the
// root and the agent creates its own worktrees; a single repo gets one fresh
// worktree at <home>/<repo>/<slug>, numbered when a known path has it.
func PlanSessionStart(ws Workspace, slug, worktreeHome string, taken []string) SessionPlan {
	if ws.Kind != WorkspaceSingle || len(ws.Repos) == 0 {
		return SessionPlan{Dir: ws.Root}
	}
	repo := ws.Repos[0]
	name := slug
	for n := 2; slices.Contains(taken, path.Join(worktreeHome, repo.Name, name)); n++ {
		name = slug + "-" + strconv.Itoa(n)
	}
	base := "HEAD"
	if repo.DefaultBranch != "" {
		base = "origin/" + repo.DefaultBranch
	}
	wt := &WorktreePlan{Repo: repo.Name, RepoPath: repo.Path, Path: path.Join(worktreeHome, repo.Name, name), Branch: name, Base: base}
	return SessionPlan{Dir: wt.Path, Worktree: wt}
}

// End is what ending a session leaves: idle, unfocused, and no longer on a
// pane, so a later pane with the same ID is not mistaken for it.
func (s Session) End() Session {
	s.State = StateIdle
	s.Pane = ""
	s.Focused = false
	return s
}

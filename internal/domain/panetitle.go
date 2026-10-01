package domain

import (
	"path"
	"slices"
	"strconv"
	"strings"
)

// AgentTitle is the strip above a session's agent pane, as in the mockup: its
// state, harness, model, effort and working directory, then one tab per
// worktree it owns with that worktree's PR.
func AgentTitle(s Session, worktrees []Worktree, cwd string) string {
	head := []string{stateMark(s.State) + " " + string(s.Harness)}
	for _, f := range []string{s.Model, s.Effort} {
		if f != "" {
			head = append(head, f)
		}
	}
	if cwd != "" {
		head = append(head, "cwd "+path.Base(cwd))
	}
	parts := []string{strings.Join(head, " · ")}
	for _, w := range worktrees {
		if slices.Contains(s.WorktreeIDs, w.ID) {
			parts = append(parts, worktreeTab(w))
		}
	}
	return strings.Join(parts, " │ ")
}

func stateMark(st AgentState) string {
	switch st {
	case StateRunning:
		return "◐"
	case StateWaiting, StatePermission:
		return "✳"
	case StateDone:
		return "●"
	}
	return "○"
}

// ShellTitle is the title row of the shell split: which worktree it runs in,
// its path, and the keys for it.
func ShellTitle(t ShellTarget) string {
	return "shell · " + t.Label + " · " + t.Dir + " · t hide · s type · T popup"
}

// worktreeName is repo:part, as the sidebar shows it; a worktree with no repo
// or part yet goes by its ID.
func worktreeName(w Worktree) string {
	part := w.SubtaskSlug
	if part == "" {
		part = w.Branch
	}
	if part == "" && w.Repo != "" && w.Path != "" {
		// why: a detached worktree has no branch; its folder still tells it apart.
		part = path.Base(w.Path)
	}
	switch {
	case w.Repo == "" && part == "":
		return w.ID
	case w.Repo == "":
		return part
	}
	return path.Base(w.Repo) + ":" + part
}

func worktreeTab(w Worktree) string {
	tab := worktreeName(w)
	pr := w.PR
	if pr == nil {
		return tab + " no PR"
	}
	tab += " #" + strconv.Itoa(pr.Number)
	switch {
	case pr.State == PRMerged:
		return tab + " merged"
	case pr.Checks == CheckPassing:
		return tab + " ✓"
	case pr.Checks == CheckFailing:
		return tab + " ✗"
	case pr.Checks == CheckPending:
		return tab + " ◐"
	}
	return tab
}

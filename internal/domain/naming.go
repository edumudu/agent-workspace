package domain

import "strings"

// NameFor picks a session name: the pinned name, then the first PR title,
// then the Linear issue title, then the task text. Blank candidates are skipped.
func NameFor(task Task, prs []PullRequest) string {
	candidates := []string{task.PinnedName}
	for _, pr := range prs {
		candidates = append(candidates, pr.Title)
	}
	candidates = append(candidates, task.IssueTitle, task.Text)
	for _, c := range candidates {
		if name := strings.TrimSpace(c); name != "" {
			return name
		}
	}
	return ""
}

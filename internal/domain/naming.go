package domain

import (
	"strings"
)

const summaryWords = 4

var (
	// why: leading politeness and request framing say nothing about the work.
	fillerWords = wordSet("please", "can", "could", "would", "you", "i", "we", "let's", "lets",
		"need", "want", "to", "help", "me", "hey", "hi", "just", "so", "also", "kindly")
	// why: a summary cut mid-phrase should not end on a connective.
	danglingWords = wordSet("the", "a", "an", "to", "of", "and", "or", "for", "in", "on", "with", "at", "by", "from", "when", "that", "is")
)

func wordSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// NameFor picks a session name: the pinned name, then the first PR title (a
// PR a worktree owns before the one the task came from), then the Linear
// issue title, then a short summary of the task text. Blank candidates are
// skipped.
func NameFor(task Task, prs []PullRequest) string {
	candidates := []string{task.PinnedName}
	for _, pr := range prs {
		candidates = append(candidates, pr.Title)
	}
	candidates = append(candidates, task.PRTitle, task.IssueTitle, SummarizeText(task.Text))
	for _, c := range candidates {
		if name := strings.TrimSpace(c); name != "" {
			return name
		}
	}
	return ""
}

// SummarizeText is the first words of the first line of a prompt, without
// its opening filler and without a trailing connective. It stops at the end
// of the first sentence.
func SummarizeText(text string) string {
	var line string
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			line = l
			break
		}
	}
	words := strings.Fields(line)
	start := 0
	for start < len(words) && fillerWords[strings.ToLower(trimWord(words[start]))] {
		start++
	}
	if start == len(words) {
		start = 0
	}
	var kept []string
	for _, w := range words[start:] {
		kept = append(kept, trimWord(w))
		if len(kept) == summaryWords || endsSentence(w) {
			break
		}
	}
	for len(kept) > 2 && danglingWords[strings.ToLower(kept[len(kept)-1])] {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, " ")
}

func trimWord(w string) string { return strings.Trim(w, "\"'.,;:!?") }

func endsSentence(w string) bool {
	return strings.HasSuffix(w, ".") || strings.HasSuffix(w, "!") || strings.HasSuffix(w, "?")
}

// PinName pins name on the task, or unpins it when name is blank.
func PinName(task Task, name string) Task {
	task.PinnedName = strings.TrimSpace(name)
	return task
}

// WithTitle records a title resolved from the work item's source: the Linear
// issue's for a Linear task, the PR's for a PR task. It reports whether the
// task changed. A pin is untouched.
func WithTitle(task Task, title string) (Task, bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		return task, false
	}
	switch task.Source {
	case TaskLinear:
		if task.IssueTitle == title {
			return task, false
		}
		task.IssueTitle = title
	case TaskPR:
		if task.PRTitle == title {
			return task, false
		}
		task.PRTitle = title
	default:
		return task, false
	}
	return task, true
}

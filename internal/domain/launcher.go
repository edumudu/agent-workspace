package domain

import (
	"slices"
	"strings"
)

const DefaultMaxParallel = 3

type LaunchItem struct {
	ID        string
	Ref       string
	URL       string
	Workspace string
	Request   StartRequest
	Starting  bool
	Err       string
}

func ParseIssueURLs(input string) (issues []Task, rejected []string) {
	seen := map[string]bool{}
	for _, tok := range strings.Fields(input) {
		t := ParseWorkItem(tok)
		if t.Source != TaskLinear {
			rejected = append(rejected, tok)
			continue
		}
		if !seen[t.Ref] {
			seen[t.Ref] = true
			issues = append(issues, t)
		}
	}
	return issues, rejected
}

func ActiveLaunched(sessions []Session, launched map[string]bool) int {
	n := 0
	for _, s := range sessions {
		if launched[s.ID] && s.Pane != "" && s.State != StateDone {
			n++
		}
	}
	return n
}

func DrainLauncher(queue []LaunchItem, active, limit int) (next, start []LaunchItem) {
	if limit <= 0 {
		limit = DefaultMaxParallel
	}
	next = slices.Clone(queue)
	for _, i := range next {
		if i.Starting {
			active++
		}
	}
	for i := range next {
		if active >= limit {
			break
		}
		if next[i].Starting || next[i].Err != "" {
			continue
		}
		next[i].Starting = true
		start = append(start, next[i])
		active++
	}
	return next, start
}

func Retarget(queue []LaunchItem, id string, req StartRequest) ([]LaunchItem, bool) {
	i := slices.IndexFunc(queue, func(x LaunchItem) bool { return x.ID == id })
	if i < 0 || queue[i].Starting || queue[i].Err != "" {
		return nil, false
	}
	next := slices.Clone(queue)
	next[i].Request = req
	return next, true
}

func Drop(queue []LaunchItem, id string) ([]LaunchItem, bool) {
	i := slices.IndexFunc(queue, func(x LaunchItem) bool { return x.ID == id })
	if i < 0 || queue[i].Starting {
		return nil, false
	}
	return slices.Delete(slices.Clone(queue), i, i+1), true
}

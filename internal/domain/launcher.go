package domain

import (
	"slices"
	"strings"
)

// DefaultMaxParallel is how many launcher sessions may work at once when the
// config sets no limit.
const DefaultMaxParallel = 3

// LaunchItem is one Linear issue waiting in the launcher queue. Starting is
// set while its session is being created, Err after that failed.
type LaunchItem struct {
	ID        string
	Ref       string
	URL       string
	Workspace string
	Request   StartRequest
	Starting  bool
	Err       string
}

// ParseIssueURLs splits input on whitespace and keeps the Linear issue URLs,
// once each and in order. Everything else, PR links and stray words included,
// comes back as rejected so the caller can say so.
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

// ActiveLaunched counts the launcher's sessions that still hold a slot: on a
// pane and not done. A session that finished frees its slot even if the user
// prompts it again later.
func ActiveLaunched(sessions []Session, launched map[string]bool) int {
	n := 0
	for _, s := range sessions {
		if launched[s.ID] && s.Pane != "" && s.State != StateDone {
			n++
		}
	}
	return n
}

// DrainLauncher marks as starting the queued items that fit in limit minus the
// sessions already active or starting. Failed items keep their place and take
// no slot. It returns the new queue and the items just marked.
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

// Retarget swaps the request of a waiting item, the way a Codex fallback
// does. An item that is starting or failed cannot change.
func Retarget(queue []LaunchItem, id string, req StartRequest) ([]LaunchItem, bool) {
	i := slices.IndexFunc(queue, func(x LaunchItem) bool { return x.ID == id })
	if i < 0 || queue[i].Starting || queue[i].Err != "" {
		return nil, false
	}
	next := slices.Clone(queue)
	next[i].Request = req
	return next, true
}

// Drop removes an item that is not starting; a session already being
// created cannot be called back.
func Drop(queue []LaunchItem, id string) ([]LaunchItem, bool) {
	i := slices.IndexFunc(queue, func(x LaunchItem) bool { return x.ID == id })
	if i < 0 || queue[i].Starting {
		return nil, false
	}
	return slices.Delete(slices.Clone(queue), i, i+1), true
}

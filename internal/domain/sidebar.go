package domain

import "sort"

type TaskGroup struct {
	Task     Task
	Sessions []Session
}

// NeedsYou is true while the agent is blocked on the user.
func (s Session) NeedsYou() bool {
	return s.State == StateWaiting || s.State == StatePermission
}

// Sidebar groups sessions under their task, in task order, leaving out ended
// sessions and tasks with no session. Sessions whose task is unknown get a
// group of their own after the known tasks. Within a group, sessions that need you come first
// and the rest keep their order.
func Sidebar(tasks []Task, sessions []Session) []TaskGroup {
	byTask := map[string][]Session{}
	var orphanOrder []string
	known := map[string]bool{}
	for _, t := range tasks {
		known[t.ID] = true
	}
	for _, s := range sessions {
		if s.Ended {
			continue
		}
		if !known[s.TaskID] && byTask[s.TaskID] == nil {
			orphanOrder = append(orphanOrder, s.TaskID)
		}
		byTask[s.TaskID] = append(byTask[s.TaskID], s)
	}
	var groups []TaskGroup
	add := func(t Task) {
		ss := byTask[t.ID]
		if len(ss) == 0 {
			return
		}
		sort.SliceStable(ss, func(i, j int) bool { return ss[i].NeedsYou() && !ss[j].NeedsYou() })
		groups = append(groups, TaskGroup{Task: t, Sessions: ss})
	}
	for _, t := range tasks {
		add(t)
	}
	for _, id := range orphanOrder {
		add(Task{ID: id})
	}
	return groups
}

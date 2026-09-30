package domain

import (
	"encoding/json"
	"time"
)

// MaxSubagents is how many subagents of one session are kept; past it the
// oldest stopped ones are dropped.
const MaxSubagents = 30

type SubagentState string

const (
	SubagentRunning SubagentState = "running"
	SubagentStopped SubagentState = "stopped"
)

// Subagent is one agent a session spawned. ParentID is empty for agents the
// session started itself, which is all Claude reports today: its hooks name
// the agent but not who spawned it.
type Subagent struct {
	SessionID string
	ID        string
	ParentID  string
	Type      string
	State     SubagentState
	StartedAt time.Time
	StoppedAt time.Time
	// Summary is the first line of the agent's last message, set on stop.
	Summary string
}

// SubagentFromHook reads a subagent start or stop from a hook payload. Other
// kinds, and payloads without an agent_id, report false. SessionID is left
// for the caller, who knows the pane.
func SubagentFromHook(kind HarnessEventKind, at time.Time, payload []byte) (Subagent, bool) {
	var p struct {
		AgentID              string `json:"agent_id"`
		AgentType            string `json:"agent_type"`
		LastAssistantMessage string `json:"last_assistant_message"`
	}
	if json.Unmarshal(payload, &p) != nil || p.AgentID == "" {
		return Subagent{}, false
	}
	sub := Subagent{ID: p.AgentID, Type: p.AgentType}
	switch kind {
	case EventSubagentStart:
		sub.State, sub.StartedAt = SubagentRunning, at
	case EventSubagentStop:
		sub.State, sub.StoppedAt = SubagentStopped, at
		sub.Summary = cutRunes(firstLine(p.LastAssistantMessage), MaxDetail)
	default:
		return Subagent{}, false
	}
	return sub, true
}

// TrackSubagent folds a start or stop into subs and returns the new list and
// the entry that changed. A stop for an agent never seen start is added as
// stopped, since hooks can be lost.
func TrackSubagent(subs []Subagent, seen Subagent) ([]Subagent, Subagent) {
	for i, s := range subs {
		if s.SessionID != seen.SessionID || s.ID != seen.ID {
			continue
		}
		if seen.Type != "" {
			s.Type = seen.Type
		}
		s.State = seen.State
		if seen.State == SubagentStopped {
			s.StoppedAt, s.Summary = seen.StoppedAt, seen.Summary
		}
		subs[i] = s
		return subs, s
	}
	if seen.State == SubagentStopped {
		seen.StartedAt = seen.StoppedAt
	}
	return dropOldestStopped(append(subs, seen), seen.SessionID), seen
}

func dropOldestStopped(subs []Subagent, sessionID string) []Subagent {
	count := 0
	for _, s := range subs {
		if s.SessionID == sessionID {
			count++
		}
	}
	for i := 0; i < len(subs) && count > MaxSubagents; {
		if subs[i].SessionID == sessionID && subs[i].State == SubagentStopped {
			subs = append(subs[:i], subs[i+1:]...)
			count--
			continue
		}
		i++
	}
	return subs
}

// EndSubagents stops every running subagent of a session, for when the
// session ends or restarts and nothing it spawned can still be running.
func EndSubagents(subs []Subagent, sessionID string, at time.Time) ([]Subagent, []Subagent) {
	var changed []Subagent
	for i, s := range subs {
		if s.SessionID == sessionID && s.State == SubagentRunning {
			s.State, s.StoppedAt = SubagentStopped, at
			subs[i] = s
			changed = append(changed, s)
		}
	}
	return subs, changed
}

// SubagentNode is a subagent placed in the tree, listed parent before
// children, each level in the order agents started.
type SubagentNode struct {
	Subagent
	Depth int
}

// SubagentTree orders one session's subagents as a tree. An agent whose
// parent is unknown is a root.
func SubagentTree(subs []Subagent) []SubagentNode {
	known := map[string]bool{}
	for _, s := range subs {
		known[s.ID] = true
	}
	var out []SubagentNode
	placed := map[string]bool{}
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, s := range subs {
			if s.ParentID != parent || placed[s.ID] {
				continue
			}
			placed[s.ID] = true
			out = append(out, SubagentNode{Subagent: s, Depth: depth})
			walk(s.ID, depth+1)
		}
	}
	for _, s := range subs {
		if s.ParentID == "" || !known[s.ParentID] {
			if !placed[s.ID] {
				placed[s.ID] = true
				out = append(out, SubagentNode{Subagent: s})
				walk(s.ID, 1)
			}
		}
	}
	for _, s := range subs {
		if !placed[s.ID] {
			placed[s.ID] = true
			out = append(out, SubagentNode{Subagent: s})
		}
	}
	return out
}

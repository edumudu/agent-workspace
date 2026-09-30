package domain

import "sort"

// Listener is one TCP listen socket as the process table reports it. Cwd is
// empty when the process's working directory could not be read.
type Listener struct {
	Port    int
	PID     int
	PGID    int
	Command string
	Cwd     string
}

// Port is a listener that belongs to a worktree.
type Port struct {
	Port    int
	PID     int
	PGID    int
	Command string
}

// PortsByWorktree maps each listener to the deepest worktree containing its
// cwd, keyed by worktree ID. Ports are sorted by port then pid, and a
// listener seen twice (IPv4 and IPv6) counts once.
func PortsByWorktree(worktrees []Worktree, listeners []Listener) map[string][]Port {
	out := map[string][]Port{}
	for _, l := range listeners {
		if l.Cwd == "" {
			continue
		}
		owner, found := "", false
		best := -1
		for _, w := range worktrees {
			if within(l.Cwd, w.Path) && len(w.Path) > best {
				owner, best, found = w.ID, len(w.Path), true
			}
		}
		if !found {
			continue
		}
		p := Port{Port: l.Port, PID: l.PID, PGID: l.PGID, Command: l.Command}
		if !containsPort(out[owner], p) {
			out[owner] = append(out[owner], p)
		}
	}
	for _, ports := range out {
		sort.Slice(ports, func(i, j int) bool {
			if ports[i].Port != ports[j].Port {
				return ports[i].Port < ports[j].Port
			}
			return ports[i].PID < ports[j].PID
		})
	}
	return out
}

func containsPort(ports []Port, p Port) bool {
	for _, x := range ports {
		if x == p {
			return true
		}
	}
	return false
}

// KillGroups keeps the requested process groups that serve a port in some
// worktree, once each and in request order. It refuses groups 1 and below and
// self, the caller's own group, so a stale or forged request cannot take
// down init or the daemon.
func KillGroups(worktrees []Worktree, requested []int, self int) []int {
	served := map[int]bool{}
	for _, w := range worktrees {
		for _, p := range w.Ports {
			served[p.PGID] = true
		}
	}
	var out []int
	seen := map[int]bool{}
	for _, g := range requested {
		if g <= 1 || g == self || !served[g] || seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, g)
	}
	return out
}

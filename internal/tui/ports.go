package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// killTimeout covers the daemon's SIGTERM grace period and the SIGKILL that
// follows it.
const killTimeout = 10 * time.Second

type killPrompt struct {
	pgids []int
	label string
}

// ports is every port of the session's worktrees.
func (e entry) ports() []domain.Port {
	var out []domain.Port
	for _, w := range e.worktrees {
		out = append(out, w.Ports...)
	}
	return out
}

// portLabel lists each port number once, sorted, as ":3000 :8081".
func portLabel(ports []domain.Port) string {
	seen := map[int]bool{}
	var numbers []int
	for _, p := range ports {
		if !seen[p.Port] {
			seen[p.Port] = true
			numbers = append(numbers, p.Port)
		}
	}
	sort.Ints(numbers)
	parts := make([]string, len(numbers))
	for i, n := range numbers {
		parts[i] = fmt.Sprintf(":%d", n)
	}
	return strings.Join(parts, " ")
}

func groupsOf(ports []domain.Port) []int {
	seen := map[int]bool{}
	var groups []int
	for _, p := range ports {
		if !seen[p.PGID] {
			seen[p.PGID] = true
			groups = append(groups, p.PGID)
		}
	}
	sort.Ints(groups)
	return groups
}

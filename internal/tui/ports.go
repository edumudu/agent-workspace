package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// why: covers the daemon's SIGTERM grace period and the SIGKILL that follows it.
const killTimeout = 10 * time.Second

type killPrompt struct {
	pgids []int
	label string
}

func (e entry) ports() []domain.Port {
	var out []domain.Port
	for _, w := range e.worktrees {
		out = append(out, w.Ports...)
	}
	return out
}

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

const rowPorts = 3

// why: a session row shares its 48 columns with the session name, so it shows
// the first few ports and counts the rest; the status line lists them all.
func rowPortLabel(ports []domain.Port) string {
	parts := strings.Fields(portLabel(ports))
	if len(parts) <= rowPorts {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts[:rowPorts], " ") + fmt.Sprintf(" +%d", len(parts)-rowPorts)
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

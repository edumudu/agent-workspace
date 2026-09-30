package domain

import (
	"reflect"
	"testing"
)

func TestPortsMapToWorktreeByCwd(t *testing.T) {
	worktrees := []Worktree{
		{ID: "/w/api-feat", Path: "/w/api-feat"},
		{ID: "/w/api-feat/nested", Path: "/w/api-feat/nested"},
		{ID: "/w/web", Path: "/w/web"},
		{ID: "/w/idle", Path: "/w/idle"},
	}
	cases := []struct {
		name      string
		listeners []Listener
		want      map[string][]Port
	}{
		{
			name:      "cwd equal to the worktree",
			listeners: []Listener{{Port: 8081, PID: 10, PGID: 10, Command: "node", Cwd: "/w/api-feat"}},
			want:      map[string][]Port{"/w/api-feat": {{Port: 8081, PID: 10, PGID: 10, Command: "node"}}},
		},
		{
			name:      "cwd in a subdirectory",
			listeners: []Listener{{Port: 3000, PID: 11, PGID: 11, Command: "bun", Cwd: "/w/web/apps/site"}},
			want:      map[string][]Port{"/w/web": {{Port: 3000, PID: 11, PGID: 11, Command: "bun"}}},
		},
		{
			name:      "the deepest worktree wins",
			listeners: []Listener{{Port: 9000, PID: 12, PGID: 12, Command: "go", Cwd: "/w/api-feat/nested/cmd"}},
			want:      map[string][]Port{"/w/api-feat/nested": {{Port: 9000, PID: 12, PGID: 12, Command: "go"}}},
		},
		{
			name:      "a sibling with a shared name prefix does not match",
			listeners: []Listener{{Port: 4000, PID: 13, PGID: 13, Command: "node", Cwd: "/w/web-old"}},
			want:      map[string][]Port{},
		},
		{
			name:      "outside every worktree",
			listeners: []Listener{{Port: 5000, PID: 14, PGID: 14, Command: "ControlCenter", Cwd: "/"}},
			want:      map[string][]Port{},
		},
		{
			name:      "no cwd is unmapped",
			listeners: []Listener{{Port: 5001, PID: 15, PGID: 15, Command: "other"}},
			want:      map[string][]Port{},
		},
		{
			name: "sorted by port, one entry per pid and port",
			listeners: []Listener{
				{Port: 9229, PID: 20, PGID: 20, Command: "node", Cwd: "/w/web"},
				{Port: 3000, PID: 20, PGID: 20, Command: "node", Cwd: "/w/web"},
				{Port: 3000, PID: 20, PGID: 20, Command: "node", Cwd: "/w/web"},
				{Port: 3000, PID: 21, PGID: 21, Command: "node", Cwd: "/w/web"},
			},
			want: map[string][]Port{"/w/web": {
				{Port: 3000, PID: 20, PGID: 20, Command: "node"},
				{Port: 3000, PID: 21, PGID: 21, Command: "node"},
				{Port: 9229, PID: 20, PGID: 20, Command: "node"},
			}},
		},
	}
	for _, c := range cases {
		if got := PortsByWorktree(worktrees, c.listeners); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestKillGroupsOnlyForListedPorts(t *testing.T) {
	worktrees := []Worktree{
		{ID: "/w/api", Ports: []Port{{Port: 8081, PID: 101, PGID: 100}, {Port: 8082, PID: 102, PGID: 100}}},
		{ID: "/w/web", Ports: []Port{{Port: 3000, PID: 201, PGID: 200}}},
		{ID: "/w/root", Ports: []Port{{Port: 80, PID: 1, PGID: 1}}},
	}
	cases := []struct {
		name      string
		requested []int
		self      int
		want      []int
	}{
		{"a listed group", []int{100}, 1234, []int{100}},
		{"one group once", []int{100, 100}, 1234, []int{100}},
		{"two groups, in request order", []int{200, 100}, 1234, []int{200, 100}},
		{"a group that serves no port is refused", []int{999}, 1234, nil},
		{"the caller's own group is refused", []int{100}, 100, nil},
		{"pid 1 and below are refused even when listed", []int{1, 0, -5}, 1234, nil},
	}
	for _, c := range cases {
		if got := KillGroups(worktrees, c.requested, c.self); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDropGroupsRemovesTheirPorts(t *testing.T) {
	w := Worktree{ID: "/w/api", Ports: []Port{
		{Port: 8081, PID: 101, PGID: 100},
		{Port: 3000, PID: 201, PGID: 200},
	}}
	got := w.WithoutGroups([]int{100})
	if want := []Port{{Port: 3000, PID: 201, PGID: 200}}; !reflect.DeepEqual(got.Ports, want) {
		t.Errorf("got %+v, want %+v", got.Ports, want)
	}
	if len(w.Ports) != 2 {
		t.Errorf("the original was changed: %+v", w.Ports)
	}
	if got := w.WithoutGroups([]int{100, 200}); got.Ports != nil {
		t.Errorf("got %+v, want no ports", got.Ports)
	}
}

package domain

import "testing"

func TestNextInViewAfterEnding(t *testing.T) {
	tasks := []Task{{ID: "t1"}, {ID: "t2"}}
	sess := func(id, task, pane string) Session { return Session{ID: id, TaskID: task, Pane: pane} }
	cases := []struct {
		name     string
		sessions []Session
		ended    string
		want     string
	}{
		{"the next row down", []Session{sess("a", "t1", "%1"), sess("b", "t1", "%2"), sess("c", "t2", "%3")}, "a", "b"},
		{"crosses into the next task group", []Session{sess("a", "t1", "%1"), sess("c", "t2", "%3")}, "a", "c"},
		{"wraps to the top from the last row", []Session{sess("a", "t1", "%1"), sess("b", "t1", "%2")}, "b", "a"},
		{"skips sessions with no pane", []Session{sess("a", "t1", "%1"), sess("b", "t1", ""), sess("c", "t2", "%3")}, "a", "c"},
		{"follows the sidebar order, not the input order", []Session{sess("z", "t2", "%9"), sess("a", "t1", "%1"), sess("b", "t1", "%2")}, "a", "b"},
		{"nothing left", []Session{sess("a", "t1", "%1"), sess("b", "t1", "")}, "a", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := NextInView(tasks, c.sessions, c.ended)
			if (c.want == "") == ok || got.ID != c.want {
				t.Fatalf("NextInView after %q = %q, %v; want %q", c.ended, got.ID, ok, c.want)
			}
		})
	}
}

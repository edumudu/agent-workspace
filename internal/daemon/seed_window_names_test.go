package daemon_test

import (
	"context"
	"testing"
)

func TestUsageDebugSeedReportsOnlyWindowsClaudeCodeSends(t *testing.T) {
	_, path := start(t, &memStore{})
	ctx := context.Background()
	c := dial(t, path)
	if err := c.Call(ctx, "debug.seed", map[string]int{"count": 3}, nil); err != nil {
		t.Fatal(err)
	}
	sub, err := dial(t, path).Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sub.State.Sessions {
		for _, l := range s.Limits {
			if l.Window != "five_hour" && l.Window != "seven_day" {
				t.Errorf("session %s has window %q, which no harness reports", s.ID, l.Window)
			}
		}
	}
}

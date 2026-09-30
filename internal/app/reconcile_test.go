package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
)

func TestReconcilePanesIdlesSessionsWhosePaneIsGone(t *testing.T) {
	tests := []struct {
		name     string
		panes    []app.PaneInfo
		bindings []app.PaneBinding
		want     []string
	}{
		{
			name:     "pane missing",
			panes:    []app.PaneInfo{{ID: "%1", Alive: true}},
			bindings: []app.PaneBinding{{SessionID: "a", Pane: "%1"}, {SessionID: "b", Pane: "%2"}},
			want:     []string{"b"},
		},
		{
			name:     "pane present but dead",
			panes:    []app.PaneInfo{{ID: "%1", Alive: false}},
			bindings: []app.PaneBinding{{SessionID: "a", Pane: "%1"}},
			want:     []string{"a"},
		},
		{
			name:     "all panes alive",
			panes:    []app.PaneInfo{{ID: "%1", Alive: true}, {ID: "%2", Alive: true}},
			bindings: []app.PaneBinding{{SessionID: "a", Pane: "%1"}, {SessionID: "b", Pane: "%2"}},
			want:     nil,
		},
		{
			name:     "tmux server gone",
			panes:    nil,
			bindings: []app.PaneBinding{{SessionID: "a", Pane: "%1"}, {SessionID: "b", Pane: "%2"}},
			want:     []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{bindings: tt.bindings}
			got, err := app.ReconcilePanes(context.Background(), fakeHost{panes: tt.panes}, store)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) || !slices.Equal(store.idled, tt.want) {
				t.Fatalf("returned %v, idled %v, want %v", got, store.idled, tt.want)
			}
		})
	}
}

func TestReconcilePanesLeavesSessionsAloneWhenHostUnreadable(t *testing.T) {
	store := &fakeStore{bindings: []app.PaneBinding{{SessionID: "a", Pane: "%1"}}}
	_, err := app.ReconcilePanes(context.Background(), fakeHost{listErr: errors.New("boom")}, store)
	if err == nil {
		t.Fatal("want error")
	}
	if len(store.idled) != 0 {
		t.Fatalf("idled %v on an unreadable host", store.idled)
	}
}

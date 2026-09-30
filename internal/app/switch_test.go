package app_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestModelSwitchTypesEachCommandAsAPasteThenEnter(t *testing.T) {
	host := &typingHost{}
	sws := []domain.Switch{{Kind: domain.SwitchModel, Value: "opus"}, {Kind: domain.SwitchEffort, Value: "high"}}
	if err := app.SendSwitches(context.Background(), host, "%3", domain.HarnessClaude, sws, 0); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"%3 paste=true /model opus", "%3 keys Enter",
		"%3 paste=true /effort high", "%3 keys Enter",
	}
	if !reflect.DeepEqual(host.typed, want) {
		t.Fatalf("typed %q", host.typed)
	}
}

func TestModelSwitchStopsAtTheFirstCommandTheHostRefuses(t *testing.T) {
	host := &typingHost{failOn: "/model opus", failErr: errors.New("pane gone")}
	sws := []domain.Switch{{Kind: domain.SwitchModel, Value: "opus"}, {Kind: domain.SwitchEffort, Value: "high"}}
	err := app.SendSwitches(context.Background(), host, "%3", domain.HarnessClaude, sws, 0)
	if err == nil || len(host.typed) != 0 {
		t.Fatalf("err %v, typed %q", err, host.typed)
	}
}

package daemon_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/giovaniif/agent-workspace/internal/adapters/claude"
	"github.com/giovaniif/agent-workspace/internal/adapters/codex"
	"github.com/giovaniif/agent-workspace/internal/daemon"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const claudeDialog = `● Bash(touch notes.txt)

────────────────────────────────────────────────────────────────────────────────
 Bash command

   touch notes.txt

 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for touch commands in /tmp/project
   3. No, and tell Claude what to do differently (esc)

 Esc to cancel
`

const codexDialog = `  Would you like to run the following command?

  $ touch notes.txt

› 1. Yes, proceed (y)
  2. Yes, and don't ask again for commands that start with ` + "`touch`" + ` (p)
  3. No, and tell Codex what to do differently (esc)

  Press enter to confirm or esc to cancel
`

func promptSetup(t *testing.T, harness domain.Harness, state domain.AgentState, screens ...string) (*rpc.Client, *fakeHost) {
	t.Helper()
	host := &fakeHost{screens: screens}
	d, path := start(t, &memStore{}, daemon.WithHarnesses(host, claude.Adapter{}, codex.Adapter{}))
	c := dial(t, path)
	sub, err := c.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.Post(daemon.SessionChanged{Session: domain.Session{ID: "a", Harness: harness, Pane: "%3", State: state}})
	next(t, sub.Diffs)
	return c, host
}

func errorCode(t *testing.T, err error) string {
	t.Helper()
	var rerr *rpc.Error
	if !errors.As(err, &rerr) {
		t.Fatalf("error %v is not an rpc error", err)
	}
	return rerr.Code
}

func TestSessionPromptReturnsTheClaudeDialogAndItsChoices(t *testing.T) {
	c, _ := promptSetup(t, domain.HarnessClaude, domain.StatePermission, claudeDialog)
	got, err := c.SessionPrompt(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	want := rpc.Prompt{
		Text: "Bash command\n\ntouch notes.txt\n\nDo you want to proceed?",
		Choices: []rpc.PromptChoice{
			{ID: "1", Label: "Yes"},
			{ID: "2", Label: "Yes, and don't ask again for touch commands in /tmp/project"},
			{ID: "3", Label: "No, and tell Claude what to do differently"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestSessionPromptReturnsTheCodexDialogAndItsChoices(t *testing.T) {
	c, _ := promptSetup(t, domain.HarnessCodex, domain.StatePermission, codexDialog)
	got, err := c.SessionPrompt(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "Would you like to run the following command?\n\n$ touch notes.txt" || len(got.Choices) != 3 || got.Choices[2].Label != "No, and tell Codex what to do differently" {
		t.Fatalf("got %+v", got)
	}
}

func TestSessionPromptIsNotFoundWhenNoDialogIsShowing(t *testing.T) {
	c, _ := promptSetup(t, domain.HarnessClaude, domain.StatePermission, "● working on it\n")
	if _, err := c.SessionPrompt(context.Background(), "a"); errorCode(t, err) != rpc.CodeNotFound {
		t.Fatalf("error %v", err)
	}
}

func TestSessionPromptIsNotFoundForAnUnknownSession(t *testing.T) {
	c, _ := promptSetup(t, domain.HarnessClaude, domain.StatePermission, claudeDialog)
	if _, err := c.SessionPrompt(context.Background(), "nope"); errorCode(t, err) != rpc.CodeNotFound {
		t.Fatalf("error %v", err)
	}
}

func TestSessionAnswerPressesTheChoicesKeysWhileThePermissionIsPending(t *testing.T) {
	c, host := promptSetup(t, domain.HarnessClaude, domain.StatePermission, claudeDialog)
	if err := c.SessionAnswer(context.Background(), "a", "3"); err != nil {
		t.Fatal(err)
	}
	if got, want := host.typedNow(), []string{"%3 keys 3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("typed %q, want %q", got, want)
	}
}

func TestSessionAnswerUsesTheCodexShortcut(t *testing.T) {
	c, host := promptSetup(t, domain.HarnessCodex, domain.StatePermission, codexDialog)
	if err := c.SessionAnswer(context.Background(), "a", "3"); err != nil {
		t.Fatal(err)
	}
	if got, want := host.typedNow(), []string{"%3 keys Escape"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("typed %q, want %q", got, want)
	}
}

func TestSessionAnswerAfterTheSessionMovedOnIsRefusedAndSendsNothing(t *testing.T) {
	for _, state := range []domain.AgentState{domain.StateRunning, domain.StateIdle, domain.StateWaiting, domain.StateDone} {
		t.Run(string(state), func(t *testing.T) {
			c, host := promptSetup(t, domain.HarnessClaude, state, claudeDialog)
			err := c.SessionAnswer(context.Background(), "a", "1")
			if errorCode(t, err) != rpc.CodeStale {
				t.Fatalf("error %v", err)
			}
			if typed := host.typedNow(); len(typed) != 0 {
				t.Fatalf("typed %q", typed)
			}
		})
	}
}

func TestSessionAnswerIsNotFoundWhenTheDialogIsGone(t *testing.T) {
	c, host := promptSetup(t, domain.HarnessClaude, domain.StatePermission, "● done\n")
	if err := c.SessionAnswer(context.Background(), "a", "1"); errorCode(t, err) != rpc.CodeNotFound {
		t.Fatalf("error %v", err)
	}
	if typed := host.typedNow(); len(typed) != 0 {
		t.Fatalf("typed %q", typed)
	}
}

func TestSessionAnswerWithAnUnknownChoiceIsABadRequestAndSendsNothing(t *testing.T) {
	c, host := promptSetup(t, domain.HarnessClaude, domain.StatePermission, claudeDialog)
	if err := c.SessionAnswer(context.Background(), "a", "9"); errorCode(t, err) != rpc.CodeBadRequest {
		t.Fatalf("error %v", err)
	}
	if typed := host.typedNow(); len(typed) != 0 {
		t.Fatalf("typed %q", typed)
	}
}

func TestSessionAnswerIsNotFoundForAnUnknownSession(t *testing.T) {
	c, _ := promptSetup(t, domain.HarnessClaude, domain.StatePermission, claudeDialog)
	if err := c.SessionAnswer(context.Background(), "nope", "1"); errorCode(t, err) != rpc.CodeNotFound {
		t.Fatalf("error %v", err)
	}
}

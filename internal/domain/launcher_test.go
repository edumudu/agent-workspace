package domain

import (
	"reflect"
	"testing"
)

func TestParseIssueURLsKeepsLinearIssuesInOrderAndReportsTheRest(t *testing.T) {
	input := "https://linear.app/acme/issue/ENG-1/first\n\n  https://linear.app/acme/issue/ENG-2/second https://github.com/acme/api/pull/3\nfix login\nhttps://linear.app/acme/issue/eng-1/again"
	issues, rejected := ParseIssueURLs(input)
	var refs []string
	for _, i := range issues {
		refs = append(refs, i.Ref)
	}
	if want := []string{"ENG-1", "ENG-2"}; !reflect.DeepEqual(refs, want) {
		t.Fatalf("issues %v, want %v", refs, want)
	}
	if issues[0].URL != "https://linear.app/acme/issue/ENG-1/first" {
		t.Fatalf("first issue %+v", issues[0])
	}
	if want := []string{"https://github.com/acme/api/pull/3", "fix", "login"}; !reflect.DeepEqual(rejected, want) {
		t.Fatalf("rejected %v, want %v", rejected, want)
	}
}

func TestParseIssueURLsOfBlankInputIsEmpty(t *testing.T) {
	issues, rejected := ParseIssueURLs(" \n\t ")
	if len(issues) != 0 || len(rejected) != 0 {
		t.Fatalf("issues %v rejected %v", issues, rejected)
	}
}

func items(ids ...string) []LaunchItem {
	out := make([]LaunchItem, len(ids))
	for i, id := range ids {
		out[i] = LaunchItem{ID: id}
	}
	return out
}

func ids(queue []LaunchItem) []string {
	var out []string
	for _, i := range queue {
		out = append(out, i.ID)
	}
	return out
}

func TestDrainLauncherStartsUpToMaxParallelInOrder(t *testing.T) {
	queue, start := DrainLauncher(items("a", "b", "c", "d", "e"), 0, 3)
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(ids(start), want) {
		t.Fatalf("started %v, want %v", ids(start), want)
	}
	starting := map[string]bool{}
	for _, i := range queue {
		starting[i.ID] = i.Starting
	}
	if want := map[string]bool{"a": true, "b": true, "c": true, "d": false, "e": false}; !reflect.DeepEqual(starting, want) {
		t.Fatalf("starting flags %v, want %v", starting, want)
	}
}

func TestDrainLauncherCountsActiveAndStartingSessionsAgainstTheMax(t *testing.T) {
	queue := items("a", "b", "c")
	queue[0].Starting = true
	rest, start := DrainLauncher(queue, 1, 3)
	if want := []string{"b"}; !reflect.DeepEqual(ids(start), want) {
		t.Fatalf("started %v, want %v", ids(start), want)
	}
	if len(rest) != 3 || rest[2].Starting {
		t.Fatalf("queue %+v", rest)
	}
}

func TestDrainLauncherLeavesFailedItemsAlone(t *testing.T) {
	queue := items("a", "b")
	queue[0].Err = "boom"
	rest, start := DrainLauncher(queue, 0, 3)
	if want := []string{"b"}; !reflect.DeepEqual(ids(start), want) {
		t.Fatalf("started %v, want %v", ids(start), want)
	}
	if rest[0].Starting || rest[0].Err != "boom" {
		t.Fatalf("failed item %+v", rest[0])
	}
}

func TestDrainLauncherDoesNotMutateTheQueueItGot(t *testing.T) {
	queue := items("a")
	DrainLauncher(queue, 0, 3)
	if queue[0].Starting {
		t.Fatal("the input queue was changed")
	}
}

func TestDrainLauncherFallsBackToTheDefaultMaxWhenUnset(t *testing.T) {
	_, start := DrainLauncher(items("a", "b", "c", "d"), 0, 0)
	if len(start) != DefaultMaxParallel {
		t.Fatalf("started %d, want %d", len(start), DefaultMaxParallel)
	}
}

func TestActiveLaunchedCountsOnlyLaunchedSessionsThatAreStillWorking(t *testing.T) {
	sessions := []Session{
		{ID: "run", Pane: "%1", State: StateRunning},
		{ID: "fresh", Pane: "%2", State: StateIdle},
		{ID: "ask", Pane: "%3", State: StateWaiting},
		{ID: "done", Pane: "%4", State: StateDone},
		{ID: "ended", Pane: "", State: StateIdle},
		{ID: "mine", Pane: "%5", State: StateRunning},
	}
	launched := map[string]bool{"run": true, "fresh": true, "ask": true, "done": true, "ended": true, "gone": true}
	if got := ActiveLaunched(sessions, launched); got != 3 {
		t.Fatalf("active %d, want 3", got)
	}
}

func TestRetargetSwapsTheRequestOfAQueuedItemOnly(t *testing.T) {
	queue := items("a", "b", "c")
	queue[1].Starting = true
	queue[2].Err = "boom"
	req := StartRequest{Harness: HarnessCodex, Model: "gpt-5", Effort: "high"}
	for id, want := range map[string]bool{"a": true, "b": false, "c": false, "z": false} {
		next, ok := Retarget(queue, id, req)
		if ok != want {
			t.Errorf("retarget %s: ok=%v, want %v", id, ok, want)
		}
		if want && next[0].Request != req {
			t.Errorf("request not swapped: %+v", next[0])
		}
	}
	if queue[0].Request != (StartRequest{}) {
		t.Fatal("the input queue was changed")
	}
}

func TestDropRemovesAnItemThatIsNotStarting(t *testing.T) {
	queue := items("a", "b", "c")
	queue[1].Starting = true
	if got, ok := Drop(queue, "a"); !ok || !reflect.DeepEqual(ids(got), []string{"b", "c"}) {
		t.Fatalf("drop a: %v %v", ids(got), ok)
	}
	if _, ok := Drop(queue, "b"); ok {
		t.Fatal("dropped an item that is starting")
	}
	if _, ok := Drop(queue, "z"); ok {
		t.Fatal("dropped an item that is not there")
	}
}

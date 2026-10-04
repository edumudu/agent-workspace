package domain

import (
	"reflect"
	"testing"
)

func TestSessionSendTextMustHaveSomethingBesidesWhitespace(t *testing.T) {
	for text, want := range map[string]bool{"": false, " \n\t": false, "go on": true, "  fix it \n": true} {
		if got := SendableText(text); got != want {
			t.Errorf("SendableText(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestSessionSendDispatchesTheSessionsOldestSendOnlyWhenItIsFree(t *testing.T) {
	queue := []QueuedSend{
		{ID: "a", Session: "other", Text: "x"},
		{ID: "b", Session: "s1", Text: "first"},
		{ID: "c", Session: "s1", Text: "second"},
	}
	cases := []struct {
		name    string
		session Session
		queue   []QueuedSend
		busy    bool
		want    string
	}{
		{"idle", Session{ID: "s1", State: StateIdle}, queue, false, "b"},
		{"done", Session{ID: "s1", State: StateDone}, queue, false, "b"},
		{"waiting", Session{ID: "s1", State: StateWaiting}, queue, false, "b"},
		{"running", Session{ID: "s1", State: StateRunning}, queue, false, ""},
		{"permission", Session{ID: "s1", State: StatePermission}, queue, false, ""},
		{"a paste in flight", Session{ID: "s1", State: StateIdle}, queue, true, ""},
		{"ended", Session{ID: "s1", State: StateIdle, Ended: true}, queue, false, ""},
		{"nothing queued for it", Session{ID: "s2", State: StateIdle}, queue, false, ""},
		{"empty queue", Session{ID: "s1", State: StateIdle}, nil, false, ""},
	}
	for _, c := range cases {
		got, ok := NextSend(c.session, c.queue, c.busy)
		if ok != (c.want != "") || got.ID != c.want {
			t.Errorf("%s: NextSend = %+v, %v; want %q", c.name, got, ok, c.want)
		}
	}
}

func TestSessionSendUnsendRemovesOnlyThatSessionsSend(t *testing.T) {
	queue := []QueuedSend{{ID: "a", Session: "s1"}, {ID: "b", Session: "s1"}, {ID: "c", Session: "s2"}}
	cases := []struct {
		name, session, id string
		want              []string
		ok                bool
	}{
		{"first", "s1", "a", []string{"b", "c"}, true},
		{"middle", "s1", "b", []string{"a", "c"}, true},
		{"another sessions id", "s2", "a", []string{"a", "b", "c"}, false},
		{"unknown", "s1", "z", []string{"a", "b", "c"}, false},
	}
	for _, c := range cases {
		before := append([]QueuedSend(nil), queue...)
		got, ok := Unsend(queue, c.session, c.id)
		if ok != c.ok || !reflect.DeepEqual(sendIDs(got), c.want) {
			t.Errorf("%s: Unsend = %v, %v; want %v, %v", c.name, sendIDs(got), ok, c.want, c.ok)
		}
		if !reflect.DeepEqual(queue, before) {
			t.Errorf("%s: Unsend changed its input to %v", c.name, sendIDs(queue))
		}
	}
}

func TestSessionSendDropSendsClearsOneSessionsQueue(t *testing.T) {
	queue := []QueuedSend{{ID: "a", Session: "s1"}, {ID: "b", Session: "s2"}, {ID: "c", Session: "s1"}}
	got, ok := DropSends(queue, "s1")
	if !ok || !reflect.DeepEqual(sendIDs(got), []string{"b"}) {
		t.Errorf("DropSends(s1) = %v, %v", sendIDs(got), ok)
	}
	if got, ok := DropSends(queue, "s3"); ok || !reflect.DeepEqual(sendIDs(got), []string{"a", "b", "c"}) {
		t.Errorf("DropSends(s3) = %v, %v", sendIDs(got), ok)
	}
	if len(queue) != 3 || queue[1].ID != "b" {
		t.Errorf("DropSends changed its input to %v", sendIDs(queue))
	}
}

func TestSessionSendRequeueGoesBackAheadOfItsSessionsOtherSends(t *testing.T) {
	back := QueuedSend{ID: "x", Session: "s1"}
	cases := []struct {
		name  string
		queue []QueuedSend
		want  []string
	}{
		{"empty", nil, []string{"x"}},
		{"behind another session", []QueuedSend{{ID: "o", Session: "s2"}, {ID: "b", Session: "s1"}}, []string{"o", "x", "b"}},
		{"only other sessions", []QueuedSend{{ID: "o", Session: "s2"}}, []string{"o", "x"}},
	}
	for _, c := range cases {
		if got := RequeueSend(c.queue, back); !reflect.DeepEqual(sendIDs(got), c.want) {
			t.Errorf("%s: RequeueSend = %v, want %v", c.name, sendIDs(got), c.want)
		}
	}
}

func sendIDs(q []QueuedSend) []string {
	out := []string{}
	for _, s := range q {
		out = append(out, s.ID)
	}
	return out
}

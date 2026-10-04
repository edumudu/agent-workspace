package domain

import (
	"encoding/base64"
	"errors"
	"reflect"
	"testing"
)

func pushKey(n int, first byte) string {
	b := make([]byte, n)
	b[0] = first
	return base64.RawURLEncoding.EncodeToString(b)
}

var (
	goodP256dh = pushKey(65, 4)
	goodAuth   = pushKey(16, 1)
)

func goodSub(endpoint string) PushSubscription {
	return PushSubscription{Endpoint: endpoint, P256dh: goodP256dh, Auth: goodAuth}
}

func TestCheckPushSubscriptionWantsAnHTTPSEndpointAndTheBrowsersKeys(t *testing.T) {
	padded := base64.URLEncoding.EncodeToString(make([]byte, 16))
	cases := []struct {
		name string
		sub  PushSubscription
		ok   bool
	}{
		{"web.push.apple.com", goodSub("https://web.push.apple.com/QGx3"), true},
		{"padded auth", PushSubscription{Endpoint: "https://fcm.googleapis.com/fcm/send/x", P256dh: goodP256dh, Auth: padded}, true},
		{"plain http", goodSub("http://web.push.apple.com/QGx3"), false},
		{"no host", goodSub("https:///QGx3"), false},
		{"not a URL", goodSub("::"), false},
		{"empty endpoint", goodSub(""), false},
		{"short p256dh", PushSubscription{Endpoint: "https://p.example/x", P256dh: pushKey(33, 4), Auth: goodAuth}, false},
		{"compressed p256dh", PushSubscription{Endpoint: "https://p.example/x", P256dh: pushKey(65, 2), Auth: goodAuth}, false},
		{"p256dh not base64", PushSubscription{Endpoint: "https://p.example/x", P256dh: "%%%", Auth: goodAuth}, false},
		{"short auth", PushSubscription{Endpoint: "https://p.example/x", P256dh: goodP256dh, Auth: pushKey(8, 1)}, false},
		{"no auth", PushSubscription{Endpoint: "https://p.example/x", P256dh: goodP256dh}, false},
	}
	for _, c := range cases {
		err := CheckPushSubscription(c.sub)
		if (err == nil) != c.ok {
			t.Errorf("%s: err %v", c.name, err)
		}
		if err != nil && !errors.Is(err, ErrPushSubscription) {
			t.Errorf("%s: %v is not ErrPushSubscription", c.name, err)
		}
	}
}

func TestSubscribePushStoresTheSubscriptionWithItsDevice(t *testing.T) {
	devices := []Device{{ID: "a"}, {ID: "b"}}
	changed, ok := SubscribePush(devices, "b", goodSub("https://p.example/1"))
	want := []Device{{ID: "b", Push: &PushSubscription{Endpoint: "https://p.example/1", P256dh: goodP256dh, Auth: goodAuth}}}
	if !ok || !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed %+v ok %v", changed, ok)
	}
	if devices[1].Push != nil {
		t.Fatal("the input slice was modified")
	}
}

func TestSubscribePushToAnUnknownDeviceChangesNothing(t *testing.T) {
	if changed, ok := SubscribePush([]Device{{ID: "a"}}, "zz", goodSub("https://p.example/1")); ok || len(changed) != 0 {
		t.Fatalf("changed %+v ok %v", changed, ok)
	}
}

func TestSubscribePushMovesAnEndpointOffTheDeviceThatHadIt(t *testing.T) {
	old := goodSub("https://p.example/1")
	devices := []Device{{ID: "old", Push: &old}, {ID: "other", Push: &PushSubscription{Endpoint: "https://p.example/2"}}, {ID: "new"}}
	changed, ok := SubscribePush(devices, "new", goodSub("https://p.example/1"))
	if !ok || len(changed) != 2 {
		t.Fatalf("changed %+v ok %v", changed, ok)
	}
	if changed[0].ID != "new" || changed[0].Push == nil || changed[0].Push.Endpoint != "https://p.example/1" {
		t.Fatalf("subscribed %+v", changed[0])
	}
	if changed[1].ID != "old" || changed[1].Push != nil {
		t.Fatalf("previous holder %+v", changed[1])
	}
}

func TestSubscribePushReplacesTheDevicesOwnSubscription(t *testing.T) {
	old := goodSub("https://p.example/1")
	changed, ok := SubscribePush([]Device{{ID: "a", Push: &old}}, "a", goodSub("https://p.example/9"))
	if !ok || len(changed) != 1 || changed[0].Push.Endpoint != "https://p.example/9" {
		t.Fatalf("changed %+v ok %v", changed, ok)
	}
}

func TestDropPushEndpointClearsOnlyTheDevicesHoldingIt(t *testing.T) {
	gone, kept := goodSub("https://p.example/gone"), goodSub("https://p.example/kept")
	devices := []Device{{ID: "a", Push: &gone}, {ID: "b", Push: &kept}, {ID: "c"}}
	changed := DropPushEndpoint(devices, "https://p.example/gone")
	if len(changed) != 1 || changed[0].ID != "a" || changed[0].Push != nil {
		t.Fatalf("changed %+v", changed)
	}
	if devices[0].Push == nil {
		t.Fatal("the input slice was modified")
	}
	if changed := DropPushEndpoint(devices, "https://p.example/none"); len(changed) != 0 {
		t.Fatalf("unknown endpoint changed %+v", changed)
	}
}

func TestPushTargetsAreTheSubscribedDevicesInOrder(t *testing.T) {
	one, two := goodSub("https://p.example/1"), goodSub("https://p.example/2")
	got := PushTargets([]Device{{ID: "a", Push: &one}, {ID: "b"}, {ID: "c", Push: &two}})
	if !reflect.DeepEqual(got, []PushSubscription{one, two}) {
		t.Fatalf("targets %+v", got)
	}
	if got := PushTargets(nil); len(got) != 0 {
		t.Fatalf("no devices gave %+v", got)
	}
}

func TestPushForCarriesTheBannerAndTheSessionsURL(t *testing.T) {
	got := PushFor(Banner{Title: "api · api@fix-login", Body: "needs permission: Bash: rm -rf dist", State: StatePermission, Group: "s1"})
	want := PushMessage{Title: "api · api@fix-login", Body: "needs permission: Bash: rm -rf dist", URL: "/#/sessions/s1", Tag: "s1"}
	if got != want {
		t.Fatalf("push %+v", got)
	}
}

func TestPushForIsNeverSilent(t *testing.T) {
	cases := []struct {
		banner      Banner
		title, body string
	}{
		{Banner{Group: "s1", State: StateDone}, "agentws", "done"},
		{Banner{Title: "  ", Body: " \n", Group: "s1", State: StateWaiting}, "agentws", "waiting"},
		{Banner{Group: "s1", State: StatePermission}, "agentws", "needs permission"},
		{Banner{Group: "s1", State: StateIdle}, "agentws", "needs you"},
	}
	for _, c := range cases {
		got := PushFor(c.banner)
		if got.Title != c.title || got.Body != c.body {
			t.Errorf("%+v gave %+v", c.banner, got)
		}
	}
}

func TestSessionURLEscapesTheID(t *testing.T) {
	cases := map[string]string{
		"s1":        "/#/sessions/s1",
		"a b/c?d#e": "/#/sessions/a%20b%2Fc%3Fd%23e",
		"":          "/",
	}
	for id, want := range cases {
		if got := SessionURL(id); got != want {
			t.Errorf("SessionURL(%q) = %q, want %q", id, got, want)
		}
	}
}

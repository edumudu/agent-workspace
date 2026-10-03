package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func sequence(values ...int) func(int) int {
	i := 0
	return func(n int) int {
		v := values[i%len(values)] % n
		i++
		return v
	}
}

func TestPairCodeIsEightCharactersFromTheInjectedSource(t *testing.T) {
	got := NewPairCode(sequence(0, 1, 2, 3, 30, 29, 8, 9))
	if got != "2345ZYAB" {
		t.Fatalf("code %q", got)
	}
	if len(NewPairCode(sequence(5))) != PairCodeLength || PairCodeLength != 8 {
		t.Fatalf("length %d", PairCodeLength)
	}
}

func TestPairCodeAlphabetHasNoLookAlikes(t *testing.T) {
	for _, c := range "01OIL" {
		if strings.ContainsRune(PairCodeAlphabet, c) {
			t.Errorf("alphabet has %q", c)
		}
	}
	if len(PairCodeAlphabet) != 31 {
		t.Errorf("alphabet has %d characters", len(PairCodeAlphabet))
	}
	seen := map[rune]bool{}
	for _, c := range PairCodeAlphabet {
		if seen[c] {
			t.Errorf("alphabet repeats %q", c)
		}
		seen[c] = true
	}
}

func TestPairCodeAsksTheSourceForTheAlphabetSize(t *testing.T) {
	NewPairCode(func(n int) int {
		if n != len(PairCodeAlphabet) {
			t.Fatalf("asked for %d", n)
		}
		return 0
	})
}

func TestNormalizePairCodeIgnoresCaseSpacesAndDashes(t *testing.T) {
	cases := map[string]string{
		" abcd-2345 ": "ABCD2345",
		"ABCD 2345":   "ABCD2345",
		"abcd2345":    "ABCD2345",
		"":            "",
	}
	for in, want := range cases {
		if got := NormalizePairCode(in); got != want {
			t.Errorf("NormalizePairCode(%q) = %q, want %q", in, got, want)
		}
	}
}

var pairT0 = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func TestPairCodeWorksOnce(t *testing.T) {
	var p Pairing
	issued := p.Issue("ABCD2345", "phone", pairT0)
	if !issued.ExpiresAt.Equal(pairT0.Add(5*time.Minute)) || issued.Name != "phone" || issued.Code != "ABCD2345" {
		t.Fatalf("issued %+v", issued)
	}
	got, err := p.Redeem("abcd-2345", "1.2.3.4", pairT0.Add(time.Second))
	if err != nil || got != issued {
		t.Fatalf("redeem: %+v, %v", got, err)
	}
	if _, err := p.Redeem("ABCD2345", "1.2.3.4", pairT0.Add(2*time.Second)); !errors.Is(err, ErrPairCodeInvalid) {
		t.Fatalf("second redeem: %v", err)
	}
}

func TestPairCodeExpiresAfterFiveMinutes(t *testing.T) {
	cases := []struct {
		after time.Duration
		ok    bool
	}{
		{5*time.Minute - time.Nanosecond, true},
		{5 * time.Minute, false},
		{time.Hour, false},
	}
	for _, c := range cases {
		var p Pairing
		p.Issue("ABCD2345", "", pairT0)
		_, err := p.Redeem("ABCD2345", "a", pairT0.Add(c.after))
		if (err == nil) != c.ok {
			t.Errorf("after %v: err %v", c.after, err)
		}
	}
}

func TestPairWrongCodesNeverVoidAValidOne(t *testing.T) {
	var p Pairing
	p.Issue("ABCD2345", "", pairT0)
	p.Issue("WXYZ6789", "", pairT0)
	for i := range 4 {
		if _, err := p.Redeem("NOPE2345", "a", pairT0.Add(time.Duration(i)*time.Second)); !errors.Is(err, ErrPairCodeInvalid) {
			t.Fatalf("wrong code %d: %v", i, err)
		}
	}
	if _, err := p.Redeem("ABCD2345", "a", pairT0.Add(10*time.Second)); err != nil {
		t.Fatalf("valid code after wrong ones: %v", err)
	}
	if _, err := p.Redeem("WXYZ6789", "b", pairT0.Add(11*time.Second)); err != nil {
		t.Fatalf("other code: %v", err)
	}
}

func TestPairRefusesASixthFailedTryInAMinuteFromOneAddress(t *testing.T) {
	var p Pairing
	p.Issue("ABCD2345", "", pairT0)
	for i := range PairFailsPerAddress {
		if _, err := p.Redeem("NOPE2345", "a", pairT0.Add(time.Duration(i)*time.Second)); !errors.Is(err, ErrPairCodeInvalid) {
			t.Fatalf("try %d: %v", i+1, err)
		}
	}
	if _, err := p.Redeem("ABCD2345", "a", pairT0.Add(30*time.Second)); !errors.Is(err, ErrPairRateLimited) {
		t.Fatalf("sixth try: %v", err)
	}
	if _, err := p.Redeem("NOPE2345", "a", pairT0.Add(59*time.Second)); !errors.Is(err, ErrPairRateLimited) {
		t.Fatalf("refused tries do not count, but the window still holds: %v", err)
	}
	if _, err := p.Redeem("ABCD2345", "a", pairT0.Add(time.Minute)); err != nil {
		t.Fatalf("the first failure left the window: %v", err)
	}
}

func TestPairLimitsAreCountedPerAddress(t *testing.T) {
	var p Pairing
	p.Issue("ABCD2345", "", pairT0)
	for range PairFailsPerAddress {
		_, _ = p.Redeem("NOPE2345", "a", pairT0)
	}
	if _, err := p.Redeem("ABCD2345", "b", pairT0); err != nil {
		t.Fatalf("another address: %v", err)
	}
}

func TestPairAllowsFiveFailuresFromOneAddress(t *testing.T) {
	var p Pairing
	p.Issue("ABCD2345", "", pairT0)
	for range PairFailsPerAddress - 1 {
		_, _ = p.Redeem("NOPE2345", "a", pairT0)
	}
	if _, err := p.Redeem("NOPE2345", "a", pairT0); !errors.Is(err, ErrPairCodeInvalid) {
		t.Fatalf("fifth try: %v", err)
	}
}

func TestPairRefusesEveryoneAfterTwentyFailuresInAMinute(t *testing.T) {
	var p Pairing
	p.Issue("ABCD2345", "", pairT0)
	for i := range PairFailsTotal {
		addr := string(rune('a' + i/PairFailsPerAddress))
		if _, err := p.Redeem("NOPE2345", addr, pairT0.Add(time.Duration(i)*time.Second)); !errors.Is(err, ErrPairCodeInvalid) {
			t.Fatalf("try %d: %v", i+1, err)
		}
	}
	if _, err := p.Redeem("ABCD2345", "z", pairT0.Add(30*time.Second)); !errors.Is(err, ErrPairRateLimited) {
		t.Fatalf("twenty-first try: %v", err)
	}
	if _, err := p.Redeem("ABCD2345", "z", pairT0.Add(time.Minute)); err != nil {
		t.Fatalf("after the window: %v", err)
	}
}

func TestPairAllowsNineteenFailuresInAllBeforeTheTwentieth(t *testing.T) {
	var p Pairing
	for i := range PairFailsTotal - 1 {
		_, _ = p.Redeem("NOPE2345", string(rune('a'+i)), pairT0)
	}
	if _, err := p.Redeem("NOPE2345", "z", pairT0); !errors.Is(err, ErrPairCodeInvalid) {
		t.Fatalf("twentieth try: %v", err)
	}
}

func TestPairLimitsMatchTheADR(t *testing.T) {
	if PairFailsPerAddress != 5 || PairFailsTotal != 20 || PairFailWindow != time.Minute || PairCodeTTL != 5*time.Minute {
		t.Fatalf("limits %d %d %v %v", PairFailsPerAddress, PairFailsTotal, PairFailWindow, PairCodeTTL)
	}
}

func TestPairForgetsExpiredCodesAndOldFailures(t *testing.T) {
	var p Pairing
	p.Issue("ABCD2345", "", pairT0)
	_, _ = p.Redeem("NOPE2345", "a", pairT0)
	p.Issue("WXYZ6789", "", pairT0.Add(10*time.Minute))
	if len(p.codes) != 1 || len(p.fails) != 0 {
		t.Fatalf("codes %v fails %v", p.codes, p.fails)
	}
}

func TestDeviceStoresOnlyTheTokensHash(t *testing.T) {
	random := make([]byte, DeviceTokenBytes)
	for i := range random {
		random[i] = byte(i)
	}
	d, token := NewDevice("dev1", "phone", random, pairT0)
	if token != "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8" {
		t.Fatalf("token %q", token)
	}
	sum := sha256.Sum256([]byte(token))
	if d.TokenHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %q", d.TokenHash)
	}
	want := Device{ID: "dev1", Name: "phone", TokenHash: d.TokenHash, Created: pairT0, LastSeen: pairT0}
	if d != want {
		t.Fatalf("device %+v", d)
	}
	if DeviceTokenBytes != 32 {
		t.Fatalf("token bytes %d", DeviceTokenBytes)
	}
}

func TestCheckDeviceTokenFindsTheDevice(t *testing.T) {
	a, ta := NewDevice("a", "", []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), pairT0)
	b, tb := NewDevice("b", "", []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), pairT0)
	devices := []Device{a, b, {ID: "blank"}}
	cases := []struct {
		token string
		want  string
		ok    bool
	}{
		{ta, "a", true},
		{tb, "b", true},
		{"nope", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := CheckDeviceToken(devices, c.token)
		if ok != c.ok || got.ID != c.want {
			t.Errorf("token %q: %+v %v", c.token, got, ok)
		}
	}
}

func TestRevokedDeviceFailsTheTokenCheck(t *testing.T) {
	a, ta := NewDevice("a", "", []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), pairT0)
	b, _ := NewDevice("b", "", []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), pairT0)
	kept, ok := RevokeDevice([]Device{a, b}, "a")
	if !ok || len(kept) != 1 || kept[0].ID != "b" {
		t.Fatalf("kept %+v %v", kept, ok)
	}
	if _, ok := CheckDeviceToken(kept, ta); ok {
		t.Fatal("revoked token still passes")
	}
	if again, ok := RevokeDevice(kept, "a"); ok || len(again) != 1 {
		t.Fatalf("unknown id: %+v %v", again, ok)
	}
}

func TestDeviceSeenIsRecordedAtMostOnceAMinute(t *testing.T) {
	d := Device{ID: "a", LastSeen: pairT0}
	cases := []struct {
		at      time.Duration
		changed bool
	}{
		{0, false},
		{time.Minute - time.Nanosecond, false},
		{time.Minute, true},
		{time.Hour, true},
	}
	for _, c := range cases {
		got, changed := d.Seen(pairT0.Add(c.at))
		if changed != c.changed {
			t.Errorf("after %v: changed %v", c.at, changed)
		}
		if c.changed && !got.LastSeen.Equal(pairT0.Add(c.at)) {
			t.Errorf("after %v: last seen %v", c.at, got.LastSeen)
		}
		if !c.changed && got != d {
			t.Errorf("after %v: device changed to %+v", c.at, got)
		}
	}
}

func TestDeviceNamePrefersTheRedeemedName(t *testing.T) {
	cases := []struct{ redeemed, issued, want string }{
		{"ipad", "phone", "ipad"},
		{"  ", "phone", "phone"},
		{"", " ", "device"},
		{" ipad ", "", "ipad"},
	}
	for _, c := range cases {
		if got := DeviceName(c.redeemed, c.issued); got != c.want {
			t.Errorf("DeviceName(%q, %q) = %q, want %q", c.redeemed, c.issued, got, c.want)
		}
	}
}

func TestNewDeviceIDSkipsTakenIDs(t *testing.T) {
	taken := map[string]bool{"22222222": true}
	got := NewDeviceID(sequence(0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1), func(id string) bool { return taken[id] })
	if got != "33333333" {
		t.Fatalf("id %q", got)
	}
}

func TestPairURLPutsTheCodeInTheFragment(t *testing.T) {
	cases := []struct {
		base, want string
		ok         bool
	}{
		{"box.example.ts.net", "https://box.example.ts.net/#pair=ABCD2345", true},
		{"https://box.example.ts.net/", "https://box.example.ts.net/#pair=ABCD2345", true},
		{" box.example.ts.net:8443/agents ", "https://box.example.ts.net:8443/agents/#pair=ABCD2345", true},
		{"http://box.example.ts.net", "", false},
		{"", "", false},
		{"https://", "", false},
	}
	for _, c := range cases {
		got, err := PairURL(c.base, "ABCD2345")
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("PairURL(%q) = %q, %v", c.base, got, err)
		}
	}
}

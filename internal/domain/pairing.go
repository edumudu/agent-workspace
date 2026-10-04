package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	PairCodeAlphabet    = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	PairCodeLength      = 8
	PairCodeTTL         = 5 * time.Minute
	PairFailWindow      = time.Minute
	PairFailsPerAddress = 5
	PairFailsTotal      = 20
	DeviceTokenBytes    = 32
	DeviceSeenEvery     = time.Minute
)

var (
	ErrPairCodeInvalid = errors.New("the pairing code is wrong or has expired")
	ErrPairRateLimited = errors.New("too many failed pairing tries; wait a minute")
	ErrPairURL         = errors.New("the public URL must be an https:// URL or a host name")
)

func randomString(intn func(int) int, alphabet string) string {
	b := make([]byte, PairCodeLength)
	for i := range b {
		b[i] = alphabet[intn(len(alphabet))]
	}
	return string(b)
}

func NewPairCode(intn func(int) int) string {
	return randomString(intn, PairCodeAlphabet)
}

func NewDeviceID(intn func(int) int, taken func(string) bool) string {
	alphabet := strings.ToLower(PairCodeAlphabet)
	for {
		if id := randomString(intn, alphabet); !taken(id) {
			return id
		}
	}
}

func NormalizePairCode(s string) string {
	s = strings.ToUpper(s)
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, s)
}

type PairCode struct {
	Code      string
	Name      string
	ExpiresAt time.Time
}

type pairFail struct {
	addr string
	at   time.Time
}

type Pairing struct {
	codes map[string]PairCode
	fails []pairFail
}

func (p *Pairing) forget(now time.Time) {
	for code, c := range p.codes {
		if !now.Before(c.ExpiresAt) {
			delete(p.codes, code)
		}
	}
	kept := p.fails[:0]
	for _, f := range p.fails {
		if now.Sub(f.at) < PairFailWindow {
			kept = append(kept, f)
		}
	}
	p.fails = kept
}

func (p *Pairing) Issue(code, name string, now time.Time) PairCode {
	p.forget(now)
	if p.codes == nil {
		p.codes = map[string]PairCode{}
	}
	c := PairCode{Code: code, Name: name, ExpiresAt: now.Add(PairCodeTTL)}
	p.codes[code] = c
	return c
}

func (p *Pairing) Redeem(code, addr string, now time.Time) (PairCode, error) {
	p.forget(now)
	if p.limited(addr) {
		return PairCode{}, ErrPairRateLimited
	}
	c, ok := p.codes[NormalizePairCode(code)]
	if !ok {
		p.fails = append(p.fails, pairFail{addr: addr, at: now})
		return PairCode{}, ErrPairCodeInvalid
	}
	delete(p.codes, c.Code)
	return c, nil
}

func (p *Pairing) limited(addr string) bool {
	if len(p.fails) >= PairFailsTotal {
		return true
	}
	n := 0
	for _, f := range p.fails {
		if f.addr == addr {
			n++
		}
	}
	return n >= PairFailsPerAddress
}

type Device struct {
	ID        string
	Name      string
	TokenHash string
	Created   time.Time
	LastSeen  time.Time
	Push      *PushSubscription
}

func HashDeviceToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewDevice(id, name string, random []byte, now time.Time) (Device, string) {
	token := base64.RawURLEncoding.EncodeToString(random)
	return Device{ID: id, Name: name, TokenHash: HashDeviceToken(token), Created: now, LastSeen: now}, token
}

func CheckDeviceToken(devices []Device, token string) (Device, bool) {
	if token == "" {
		return Device{}, false
	}
	hash := []byte(HashDeviceToken(token))
	for _, d := range devices {
		if subtle.ConstantTimeCompare(hash, []byte(d.TokenHash)) == 1 {
			return d, true
		}
	}
	return Device{}, false
}

func RevokeDevice(devices []Device, id string) ([]Device, bool) {
	kept := make([]Device, 0, len(devices))
	for _, d := range devices {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	return kept, len(kept) < len(devices)
}

func (d Device) Seen(now time.Time) (Device, bool) {
	if now.Sub(d.LastSeen) < DeviceSeenEvery {
		return d, false
	}
	d.LastSeen = now
	return d, true
}

func DeviceName(redeemed, issued string) string {
	for _, n := range []string{redeemed, issued} {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
	}
	return "device"
}

func PairURL(base, code string) (string, error) {
	base = strings.TrimSpace(base)
	if strings.HasPrefix(base, "http://") {
		return "", ErrPairURL
	}
	base = strings.TrimSuffix(strings.TrimPrefix(base, "https://"), "/")
	if base == "" || strings.Contains(base, "://") || strings.Contains(base, "#") {
		return "", ErrPairURL
	}
	return "https://" + base + "/#pair=" + code, nil
}

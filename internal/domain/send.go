package domain

import (
	"slices"
	"strings"
	"time"
)

type QueuedSend struct {
	ID       string    `json:"id"`
	Session  string    `json:"session"`
	Text     string    `json:"text"`
	QueuedAt time.Time `json:"queued_at"`
}

func SendableText(text string) bool { return strings.TrimSpace(text) != "" }

func NextSend(s Session, queue []QueuedSend, busy bool) (QueuedSend, bool) {
	if busy || s.Ended || !s.AcceptsSwitch() {
		return QueuedSend{}, false
	}
	for _, q := range queue {
		if q.Session == s.ID {
			return q, true
		}
	}
	return QueuedSend{}, false
}

func Unsend(queue []QueuedSend, session, id string) ([]QueuedSend, bool) {
	i := slices.IndexFunc(queue, func(q QueuedSend) bool { return q.Session == session && q.ID == id })
	if i < 0 {
		return queue, false
	}
	return slices.Delete(slices.Clone(queue), i, i+1), true
}

func DropSends(queue []QueuedSend, session string) ([]QueuedSend, bool) {
	kept := slices.DeleteFunc(slices.Clone(queue), func(q QueuedSend) bool { return q.Session == session })
	return kept, len(kept) != len(queue)
}

func RequeueSend(queue []QueuedSend, back QueuedSend) []QueuedSend {
	i := slices.IndexFunc(queue, func(q QueuedSend) bool { return q.Session == back.Session })
	if i < 0 {
		i = len(queue)
	}
	return slices.Insert(slices.Clone(queue), i, back)
}

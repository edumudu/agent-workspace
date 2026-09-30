package claude

import (
	"encoding/json"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// Event maps a Claude hook, by name and stdin payload, to the harness event.
// It is pure and cheap, so the daemon runs it on its loop.
func Event(name string, payload []byte) (domain.HarnessEventKind, bool) {
	if name != "Notification" {
		return domain.HookEvent(domain.HarnessClaude, name)
	}
	var n struct {
		NotificationType string `json:"notification_type"`
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &n)
	}
	return domain.ClaudeNotification(n.NotificationType)
}

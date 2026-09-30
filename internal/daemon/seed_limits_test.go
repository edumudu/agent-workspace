package daemon_test

import (
	"context"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

func TestUsageDebugSeedGivesEachHarnessFreshLimitsIncludingALowOne(t *testing.T) {
	_, path := start(t, &memStore{})
	ctx := context.Background()
	if err := dial(t, path).DebugSeed(ctx, 2); err != nil {
		t.Fatal(err)
	}
	sub, err := dial(t, path).Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	quotas := domain.Quotas(sub.State.Sessions)
	seen := map[domain.Harness]int{}
	low := false
	for _, q := range quotas {
		seen[q.Harness]++
		low = low || q.Low()
		if q.Stale(time.Now()) {
			t.Errorf("seeded quota is stale: %+v", q)
		}
	}
	if seen[domain.HarnessClaude] < 2 || seen[domain.HarnessCodex] < 2 || !low {
		t.Fatalf("quotas %+v", quotas)
	}
}

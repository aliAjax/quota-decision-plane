package application

import (
	"context"
	"testing"
	"time"

	mem "github.com/enterprise-labs/quota-decision-plane/internal/counter/infrastructure"
)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

func TestCompareAndAdjustOnlyBuffersOnSuccess(t *testing.T) {
	store := mem.NewMemory()
	ctx := context.Background()
	if _, err := store.Adjust(ctx, "k", 5, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	ledger := NewLedger(store, &fixedClock{now: time.Unix(10, 0)}, 100)
	_, swapped, err := ledger.CompareAndAdjust(ctx, "k", 99, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if swapped {
		t.Fatal("expected swap to fail")
	}
	if got := ledger.PendingEvents(); got != 0 {
		t.Fatalf("pending=%d, want 0", got)
	}
}

func TestCompactKeepsRecentEvents(t *testing.T) {
	store := mem.NewMemory()
	ctx := context.Background()
	clock := &fixedClock{now: time.Unix(100, 0)}
	ledger := NewLedger(store, clock, 100)
	if _, err := ledger.Adjust(ctx, "k", 1, 0); err != nil {
		t.Fatal(err)
	}
	clock.now = time.Unix(200, 0)
	if _, err := ledger.Adjust(ctx, "k", 1, 0); err != nil {
		t.Fatal(err)
	}
	compacted := ledger.Compact(time.Unix(150, 0))
	if len(compacted) != 1 {
		t.Fatalf("compacted=%d, want 1", len(compacted))
	}
	if compacted[0].FirstAt.Unix() != 100 {
		t.Fatalf("compacted first=%d, want 100", compacted[0].FirstAt.Unix())
	}
	if got := ledger.PendingEvents(); got != 1 {
		t.Fatalf("pending=%d, want 1", got)
	}
}

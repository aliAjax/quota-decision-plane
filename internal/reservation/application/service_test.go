package application

import (
	"context"
	clockpkg "github.com/enterprise-labs/quota-decision-plane/internal/platform/domain"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
	reservation "github.com/enterprise-labs/quota-decision-plane/internal/reservation/domain"
	mem "github.com/enterprise-labs/quota-decision-plane/internal/reservation/infrastructure"
	"testing"
	"time"
)

func TestCancelAndExpireRelease(t *testing.T) {
	ctx := context.Background()
	clock := &clockpkg.ManualClock{Current: time.Unix(1, 0)}
	released := int64(0)
	service := NewService(mem.NewMemoryRepository(), clock, func(items []quota.Allocation) {
		for _, x := range items {
			released += x.Cost
		}
	})
	makeItem := func(id string) reservation.Reservation {
		return reservation.Reservation{ID: id, Cost: 2, ExpiresAt: clock.Now().Add(time.Second), Allocations: []quota.Allocation{{Cost: 2}, {Cost: 2}}}
	}
	if err := service.Create(ctx, makeItem("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Cancel(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if released != 4 {
		t.Fatalf("released=%d", released)
	}
	if err := service.Create(ctx, makeItem("b")); err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Second)
	count, err := service.Reap(ctx)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if released != 8 {
		t.Fatalf("released after expiry=%d", released)
	}
}

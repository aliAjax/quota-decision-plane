package application

import (
	"context"
	"errors"
	"testing"
	"time"

	clockpkg "github.com/enterprise-labs/quota-decision-plane/internal/platform/domain"
	reservation "github.com/enterprise-labs/quota-decision-plane/internal/reservation/domain"
	mem "github.com/enterprise-labs/quota-decision-plane/internal/reservation/infrastructure"
)

func TestReservationGetPreservesNotFound(t *testing.T) {
	service := NewService(mem.NewMemoryRepository(), &clockpkg.ManualClock{Current: time.Unix(1, 0)}, nil)
	_, err := service.Get(context.Background(), "missing")
	if !errors.Is(err, reservation.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReservationCommitPreservesInvalidTransition(t *testing.T) {
	ctx := context.Background()
	clock := &clockpkg.ManualClock{Current: time.Unix(1, 0)}
	repo := mem.NewMemoryRepository()
	service := NewService(repo, clock, nil)
	item := reservation.Reservation{ID: "a", Cost: 1, Status: reservation.Pending, ExpiresAt: clock.Now().Add(-time.Second)}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	_, err := service.Commit(ctx, "a")
	if !errors.Is(err, reservation.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestReservationCancelPreservesInvalidTransition(t *testing.T) {
	ctx := context.Background()
	clock := &clockpkg.ManualClock{Current: time.Unix(1, 0)}
	repo := mem.NewMemoryRepository()
	service := NewService(repo, clock, nil)
	item := reservation.Reservation{ID: "a", Cost: 1, Status: reservation.Committed, ExpiresAt: clock.Now().Add(time.Minute)}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	_, err := service.Cancel(ctx, "a")
	if !errors.Is(err, reservation.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

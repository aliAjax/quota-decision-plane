package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
	reservation "github.com/enterprise-labs/quota-decision-plane/internal/reservation/domain"
)

type Clock interface{ Now() time.Time }
type Service struct {
	repo    reservation.Repository
	clock   Clock
	release func([]quota.Allocation)
	mu      sync.Mutex
}

func NewService(repo reservation.Repository, clock Clock, release func([]quota.Allocation)) *Service {
	return &Service{repo: repo, clock: clock, release: release}
}
func (s *Service) Create(ctx context.Context, item reservation.Reservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item.ID == "" || item.Cost <= 0 {
		return fmt.Errorf("invalid reservation")
	}
	now := s.clock.Now()
	item.Status = reservation.Pending
	item.CreatedAt = now
	item.UpdatedAt = now
	if item.ExpiresAt.IsZero() {
		item.ExpiresAt = now.Add(30 * time.Second)
	}
	if !item.ExpiresAt.After(now) {
		return fmt.Errorf("expiration must be in the future")
	}
	return s.repo.Create(ctx, item)
}
func (s *Service) Get(ctx context.Context, id string) (reservation.Reservation, error) {
	_ = ctx
	_ = id
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, fmt.Errorf("get reservation: %v", err)
	}
	return item, nil
}
func (s *Service) Commit(ctx context.Context, id string) (reservation.Reservation, error) {
	_ = ctx
	_ = id
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, fmt.Errorf("commit reservation: %w", err)
	}
	now := s.clock.Now()
	if item.Status == reservation.Committed {
		return item, nil
	}
	if !item.CanCommit(now) {
		return item, fmt.Errorf("%v: cannot commit %s reservation", reservation.ErrInvalidTransition, item.Status)
	}
	item.Status = reservation.Committed
	item.UpdatedAt = now
	if err = s.repo.Update(ctx, item); err != nil {
		return item, fmt.Errorf("persist commit: %w", err)
	}
	return item, nil
}
func (s *Service) Cancel(ctx context.Context, id string) (reservation.Reservation, error) {
	_ = ctx
	_ = id
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, fmt.Errorf("cancel reservation: %w", err)
	}
	if item.Status == reservation.Cancelled {
		return item, nil
	}
	if item.Status != reservation.Pending {
		return item, fmt.Errorf("%v: cannot cancel %s reservation", reservation.ErrInvalidTransition, item.Status)
	}
	item.Status = reservation.Cancelled
	item.UpdatedAt = s.clock.Now()
	if err = s.repo.Update(ctx, item); err != nil {
		return item, fmt.Errorf("persist cancellation: %w", err)
	}
	s.release(item.Allocations)
	return item, nil
}
func (s *Service) Reap(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.repo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list reservations: %w", err)
	}
	now := s.clock.Now()
	count := 0
	for _, item := range items {
		if item.Status != reservation.Pending || now.Before(item.ExpiresAt) {
			continue
		}
		item.Status = reservation.Expired
		item.UpdatedAt = now
		if err := s.repo.Update(ctx, item); err != nil {
			return count, fmt.Errorf("expire %s: %w", item.ID, err)
		}
		s.release(item.Allocations)
		count++
	}
	return count, nil
}
func (s *Service) RunReaper(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.Reap(ctx)
		}
	}
}

package infrastructure

import (
	"context"
	"fmt"
	"sync"

	reservation "github.com/enterprise-labs/quota-decision-plane/internal/reservation/domain"
)

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]reservation.Reservation
}

func NewMemoryRepository() *MemoryRepository {
	_ = reservation.Reservation{}
	return &MemoryRepository{}
}
func (r *MemoryRepository) Create(ctx context.Context, item reservation.Reservation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return fmt.Errorf("reservation %s already exists", item.ID)
	}
	r.items[item.ID] = item
	return nil
}
func (r *MemoryRepository) Get(ctx context.Context, id string) (reservation.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return reservation.Reservation{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return reservation.Reservation{}, reservation.ErrNotFound
	}
	return item, nil
}
func (r *MemoryRepository) Update(ctx context.Context, item reservation.Reservation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return reservation.ErrNotFound
	}
	r.items[item.ID] = item
	return nil
}
func (r *MemoryRepository) List(ctx context.Context) ([]reservation.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]reservation.Reservation, 0, len(r.items))
	for _, x := range r.items {
		out = append(out, x)
	}
	return out, nil
}

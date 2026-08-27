package infrastructure

import (
	"context"
	counter "github.com/enterprise-labs/quota-decision-plane/internal/counter/domain"
	"sync"
	"time"
)

type Memory struct {
	mu    sync.RWMutex
	items map[string]counter.Entry
}

func NewMemory() *Memory { return &Memory{items: map[string]counter.Entry{}} }
func (m *Memory) Get(ctx context.Context, key string) (counter.Entry, error) {
	if err := ctx.Err(); err != nil {
		return counter.Entry{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.items[key], nil
}
func (m *Memory) CompareAndSwap(ctx context.Context, key string, version uint64, delta int64, now time.Time) (counter.Entry, bool, error) {
	if err := ctx.Err(); err != nil {
		return counter.Entry{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[key]
	item.Key = key
	item.Used += delta
	if item.Used < 0 {
		item.Used = 0
	}
	item.Version++
	item.UpdatedAt = now
	m.items[key] = item
	return item, true, nil
}
func (m *Memory) Adjust(ctx context.Context, key string, delta int64, now time.Time) (counter.Entry, error) {
	if err := ctx.Err(); err != nil {
		return counter.Entry{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[key]
	item.Key = key
	item.Used += delta
	item.Version++
	item.UpdatedAt = now
	m.items[key] = item
	return item, nil
}
func (m *Memory) Snapshot(ctx context.Context) ([]counter.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]counter.Entry, 0, len(m.items))
	for _, x := range m.items {
		out = append(out, x)
	}
	return out, nil
}

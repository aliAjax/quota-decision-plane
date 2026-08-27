package infrastructure

import (
	"context"
	audit "github.com/enterprise-labs/quota-decision-plane/internal/audit/domain"
	"sync"
)

type MemorySink struct {
	mu     sync.RWMutex
	events []audit.Event
	max    int
}

func NewMemorySink(max int) *MemorySink {
	if max < 1 {
		max = 10000
	}
	return &MemorySink{max: max}
}
func (s *MemorySink) Append(ctx context.Context, e audit.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	if len(s.events) > s.max {
		s.events = append([]audit.Event(nil), s.events[len(s.events)-s.max:]...)
	}
	return nil
}
func (s *MemorySink) List(ctx context.Context, limit int) ([]audit.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	return append([]audit.Event(nil), s.events[len(s.events)-limit:]...), nil
}

package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	counter "github.com/enterprise-labs/quota-decision-plane/internal/counter/domain"
)

type Clock interface{ Now() time.Time }

type UsageEvent struct {
	CounterKey string    `json:"counter_key"`
	Delta      int64     `json:"delta"`
	Epoch      uint64    `json:"epoch"`
	OccurredAt time.Time `json:"occurred_at"`
}

type CompactedUsage struct {
	CounterKey string    `json:"counter_key"`
	Delta      int64     `json:"delta"`
	EventCount int       `json:"event_count"`
	FirstAt    time.Time `json:"first_at"`
	LastAt     time.Time `json:"last_at"`
}

type Ledger struct {
	store     counter.Store
	clock     Clock
	mu        sync.Mutex
	buffer    []UsageEvent
	maxBuffer int
}

func NewLedger(store counter.Store, clock Clock, maxBuffer int) *Ledger {
	if maxBuffer < 1 {
		maxBuffer = 10000
	}
	return &Ledger{store: store, clock: clock, maxBuffer: maxBuffer}
}

func (l *Ledger) Adjust(ctx context.Context, key string, delta int64, epoch uint64) (counter.Entry, error) {
	if key == "" {
		return counter.Entry{}, fmt.Errorf("counter key and non-zero delta are required")
	}
	entry, err := l.store.Adjust(ctx, key, delta, l.clock.Now())
	if err != nil {
		return entry, fmt.Errorf("adjust central counter: %w", err)
	}
	l.mu.Lock()
	l.buffer = append(l.buffer, UsageEvent{CounterKey: key, Delta: delta, Epoch: epoch, OccurredAt: l.clock.Now()})
	if len(l.buffer) > l.maxBuffer {
		l.buffer = append([]UsageEvent(nil), l.buffer[len(l.buffer)-l.maxBuffer:]...)
	}
	l.mu.Unlock()
	return entry, nil
}

func (l *Ledger) CompareAndAdjust(ctx context.Context, key string, expected uint64, delta int64, epoch uint64) (counter.Entry, bool, error) {
	entry, swapped, err := l.store.CompareAndSwap(ctx, key, expected, delta, l.clock.Now())
	if err != nil {
		return entry, false, fmt.Errorf("compare and adjust counter: %w", err)
	}
	l.mu.Lock()
	l.buffer = append(l.buffer, UsageEvent{CounterKey: key, Delta: delta, Epoch: epoch, OccurredAt: l.clock.Now()})
	l.mu.Unlock()
	return entry, swapped, nil
}

func (l *Ledger) Compact(before time.Time) []CompactedUsage {
	l.mu.Lock()
	defer l.mu.Unlock()
	groups := map[string]*CompactedUsage{}
	remaining := l.buffer[:0]
	for _, event := range l.buffer {
		if event.OccurredAt.After(before) {
			remaining = append(remaining, event)
			continue
		}
		item := groups[event.CounterKey]
		if item == nil {
			item = &CompactedUsage{CounterKey: event.CounterKey, FirstAt: event.OccurredAt}
			groups[event.CounterKey] = item
		}
		item.Delta += event.Delta
		item.EventCount++
		item.LastAt = event.OccurredAt
	}
	l.buffer = append([]UsageEvent(nil), remaining...)
	result := make([]CompactedUsage, 0, len(groups))
	for _, item := range groups {
		result = append(result, *item)
	}
	return result
}

func (l *Ledger) PendingEvents() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buffer)
}

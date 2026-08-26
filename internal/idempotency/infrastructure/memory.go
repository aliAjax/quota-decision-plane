package infrastructure

import (
	"context"
	idem "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/domain"
	"sync"
	"time"
)

type Memory struct {
	mu      sync.RWMutex
	records map[string]idem.Record
}

func NewMemory() *Memory { return &Memory{records: map[string]idem.Record{}} }
func (m *Memory) Get(ctx context.Context, key string) (idem.Record, bool, error) {
	if err := ctx.Err(); err != nil {
		return idem.Record{}, false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.records[key]
	if ok && !r.ExpiresAt.IsZero() && time.Now().After(r.ExpiresAt) {
		return idem.Record{}, false, nil
	}
	r.Body = cloneBytes(r.Body)
	return r, ok, nil
}
func (m *Memory) Put(ctx context.Context, r idem.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r.Body = cloneBytes(r.Body)
	m.records[r.Key] = r
	return nil
}
func (m *Memory) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.records {
		if !r.ExpiresAt.IsZero() && now.After(r.ExpiresAt) {
			delete(m.records, r.Key)
			n++
		}
	}
	return n, nil
}

func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

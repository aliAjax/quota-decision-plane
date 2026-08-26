package application

import (
	"fmt"
	lease "github.com/enterprise-labs/quota-decision-plane/internal/lease/domain"
	"sync"
	"time"
)

type Clock interface{ Now() time.Time }
type Manager struct {
	mu     sync.Mutex
	clock  Clock
	items  map[uint32]lease.Lease
	epochs map[uint32]uint64
}

func NewManager(clock Clock) *Manager {
	return &Manager{clock: clock, items: map[uint32]lease.Lease{}, epochs: map[uint32]uint64{}}
}
func (m *Manager) Acquire(shard uint32, owner string, ttl time.Duration) (lease.Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	current, ok := m.items[shard]
	if ok && now.Before(current.ExpiresAt) && current.Owner != owner {
		return current, fmt.Errorf("%w: shard %d owner %s", lease.ErrLeaseHeld, shard, current.Owner)
	}
	if !ok || current.Owner != owner || !now.Before(current.ExpiresAt) {
		m.epochs[shard]++
	}
	next := lease.Lease{Shard: shard, Owner: owner, Epoch: m.epochs[shard], ExpiresAt: now.Add(ttl)}
	m.items[shard] = next
	return next, nil
}
func (m *Manager) Renew(shard uint32, owner string, epoch uint64, ttl time.Duration) (lease.Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[shard]
	if !ok || m.clock.Now().After(current.ExpiresAt) {
		return current, lease.ErrLeaseExpired
	}
	if current.Owner != owner || current.Epoch != epoch {
		return current, lease.ErrStaleEpoch
	}
	current.ExpiresAt = m.clock.Now().Add(ttl)
	m.items[shard] = current
	return current, nil
}
func (m *Manager) Validate(shard uint32, owner string, epoch uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[shard]
	if !ok || m.clock.Now().After(current.ExpiresAt) {
		return lease.ErrLeaseExpired
	}
	if current.Owner != owner || current.Epoch != epoch {
		return lease.ErrStaleEpoch
	}
	return nil
}
func (m *Manager) Release(shard uint32, owner string, epoch uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[shard]
	if !ok {
		return nil
	}
	if current.Owner != owner || current.Epoch != epoch {
		return lease.ErrStaleEpoch
	}
	delete(m.items, shard)
	return nil
}
func (m *Manager) List() []lease.Lease {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]lease.Lease, 0, len(m.items))
	for _, l := range m.items {
		out = append(out, l)
	}
	return out
}

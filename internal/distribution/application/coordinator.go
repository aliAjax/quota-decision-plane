package application

import (
	"errors"
	"fmt"
	dist "github.com/enterprise-labs/quota-decision-plane/internal/distribution/domain"
	"sync"
	"time"
)

var ErrTokensExhausted = errors.New("local token lease exhausted")

type Clock interface{ Now() time.Time }
type Coordinator struct {
	mu             sync.Mutex
	clock          Clock
	leases         map[string]dist.TokenLease
	corrections    []dist.Correction
	maxCorrections int
}

func NewCoordinator(clock Clock) *Coordinator {
	return &Coordinator{clock: clock, leases: map[string]dist.TokenLease{}, maxCorrections: 1000}
}
func (c *Coordinator) Grant(id, node, key string, amount int64, epoch uint64, ttl time.Duration) (dist.TokenLease, error) {
	if amount <= 0 {
		return dist.TokenLease{}, fmt.Errorf("amount must be positive")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	lease := dist.TokenLease{ID: id, NodeID: node, CounterKey: key, Granted: amount, Epoch: epoch, ExpiresAt: c.clock.Now().Add(ttl)}
	c.leases[id] = lease
	return lease, nil
}
func (c *Coordinator) Consume(id string, cost int64, epoch uint64) (dist.TokenLease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.leases[id]
	if !ok {
		return l, fmt.Errorf("token lease not found")
	}
	if c.clock.Now().After(l.ExpiresAt) {
		return l, fmt.Errorf("token lease expired")
	}
	if l.Epoch != epoch {
		return l, fmt.Errorf("fencing epoch mismatch")
	}
	if l.Consumed+cost > l.Granted {
		return l, ErrTokensExhausted
	}
	l.Consumed += cost
	c.leases[id] = l
	return l, nil
}
func (c *Coordinator) Reconcile(key string, central, reported int64) dist.Correction {
	c.mu.Lock()
	defer c.mu.Unlock()
	item := dist.Correction{CounterKey: key, CentralUsed: central, ReportedUsed: reported, Delta: reported - central, AppliedAt: c.clock.Now()}
	c.corrections = append(c.corrections, item)
	if len(c.corrections) > c.maxCorrections {
		c.corrections = append([]dist.Correction(nil), c.corrections[len(c.corrections)-c.maxCorrections:]...)
	}
	return item
}
func (c *Coordinator) ConservativeAllowance(limit, used, cost, maxOverage int64, nodeHealthy bool) bool {
	if !nodeHealthy {
		return used+cost <= limit
	}
	return used+cost <= limit+maxOverage
}
func (c *Coordinator) Corrections() []dist.Correction {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]dist.Correction(nil), c.corrections...)
}

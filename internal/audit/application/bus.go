package application

import (
	"context"
	audit "github.com/enterprise-labs/quota-decision-plane/internal/audit/domain"
	"sync"
	"sync/atomic"
)

type Bus struct {
	queue   chan audit.Event
	sink    audit.Sink
	dropped atomic.Uint64
	mu      sync.RWMutex
	closed  bool
	stop    sync.Once
	done    chan struct{}
}

func NewBus(sink audit.Sink, size int) *Bus {
	if size < 1 {
		size = 1024
	}
	return &Bus{queue: make(chan audit.Event, size), sink: sink, done: make(chan struct{})}
}
func (b *Bus) Publish(e audit.Event) {
	select {
	case b.queue <- e:
	default:
		b.dropped.Add(1)
	}
}
func (b *Bus) Run(ctx context.Context) {
	defer close(b.done)
	for {
		b.mu.RLock()
		closed := b.closed
		b.mu.RUnlock()
		if closed {
			return
		}
		select {
		case <-ctx.Done():
			return
		case e, ok := <-b.queue:
			if !ok {
				return
			}
			_ = b.sink.Append(context.Background(), e)
		}
	}
}
func (b *Bus) Close(ctx context.Context) error {
	b.stop.Do(func() {
		b.mu.Lock()
		b.closed = true
		close(b.queue)
		b.mu.Unlock()
	})
	return nil
}
func (b *Bus) Closed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.closed
}
func (b *Bus) Dropped() uint64 { return b.dropped.Load() }

package application

import (
	"context"
	audit "github.com/enterprise-labs/quota-decision-plane/internal/audit/domain"
	"sync/atomic"
)

type Bus struct {
	queue   chan audit.Event
	sink    audit.Sink
	dropped atomic.Uint64
}

func NewBus(sink audit.Sink, size int) *Bus {
	if size < 1 {
		size = 1024
	}
	return &Bus{queue: make(chan audit.Event, size), sink: sink}
}
func (b *Bus) Publish(e audit.Event) {
	select {
	case b.queue <- e:
	default:
		b.dropped.Add(1)
	}
}
func (b *Bus) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-b.queue:
			_ = b.sink.Append(context.Background(), e)
		}
	}
}
func (b *Bus) Dropped() uint64 { return b.dropped.Load() }

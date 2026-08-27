package application

import (
	"context"
	audit "github.com/enterprise-labs/quota-decision-plane/internal/audit/domain"
	"sync/atomic"
)

// Bus is a non-blocking audit event bus. Producers call Publish from the
// hot request path; a single consumer goroutine (Run) drains the queue and
// appends to the sink.
//
// Shutdown contract:
//
//	Close signals Run to stop accepting new events, drain everything already
//	queued, and exit. Close blocks until the drain finishes or ctx expires.
//	Publish remains safe to call at any point in the lifecycle — including
//	concurrently with Close and after Close returns — and never panics; once
//	the bus is closing it reports the event as dropped instead.
//
// The queue channel is never closed. Closing a channel that concurrent
// senders write to is a data race and panics on send, so the bus uses an
// atomic closing flag both to gate Publish and to tell Run when to exit.
type Bus struct {
	queue   chan audit.Event
	sink    audit.Sink
	dropped atomic.Uint64
	closing atomic.Bool
	stop    chan struct{}
	done    chan struct{}
}

func NewBus(sink audit.Sink, size int) *Bus {
	if size < 1 {
		size = 1024
	}
	return &Bus{
		queue: make(chan audit.Event, size),
		sink:  sink,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Publish enqueues an event without blocking the caller. If the queue is full
// or the bus is closing/closed, the event is counted as dropped rather than
// blocking or panicking. Safe for concurrent use and for concurrent/after Close.
func (b *Bus) Publish(e audit.Event) {
	if b.closing.Load() {
		b.dropped.Add(1)
		return
	}
	select {
	case b.queue <- e:
	default:
		b.dropped.Add(1)
	}
}

// Run consumes the queue and appends events to the sink until Close requests
// shutdown and the queue has been fully drained. It is the only goroutine that
// reads from queue, and it owns the sink interaction. Run returns once the
// drain completes; Close blocks on <-done for that signal.
//
// Run deliberately ignores ctx cancellation as an exit trigger. In production
// the signal context is canceled the instant shutdown begins, but the audit
// drain must run to completion *during* shutdown — so Close, not ctx, owns the
// lifecycle. ctx is plumbed to the sink so a sink may still observe deadlines.
func (b *Bus) Run(ctx context.Context) {
	defer close(b.done)
	for {
		select {
		case <-b.stop:
			// Drain the backlog that accumulated before producers observed
			// closing; then exit. New Publish calls now report dropped, so no
			// further sends can arrive after this drain runs to completion.
			b.drain(ctx)
			return
		case e := <-b.queue:
			_ = b.sink.Append(ctx, e)
		}
	}
}

// drain flushes whatever remains in the queue without blocking on an empty
// channel. It is bounded by the queue depth, so it always terminates.
func (b *Bus) drain(ctx context.Context) {
	for {
		select {
		case e, ok := <-b.queue:
			if !ok {
				return
			}
			_ = b.sink.Append(context.Background(), e)
		default:
			return
		}
	}
}

// Close requests shutdown of the consumer, waits for Run to finish draining the
// queue, and returns nil when the drain is complete. If ctx expires before the
// drain finishes, Close returns ctx.Err() while still letting Run finish in the
// background. Idempotent: the first call drives shutdown, later calls just wait.
func (b *Bus) Close(ctx context.Context) error {
	if b.closing.Swap(true) {
		// Another caller already initiated shutdown; just wait for completion.
		select {
		case <-b.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	close(b.stop)
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Closed reports whether Close has begun. Events published after this becomes
// true are counted as dropped. Safe for concurrent use.
func (b *Bus) Closed() bool { return b.closing.Load() }

// Dropped returns the count of events that could not be enqueued because the
// queue was full or the bus was closing/closed.
func (b *Bus) Dropped() uint64 { return b.dropped.Load() }
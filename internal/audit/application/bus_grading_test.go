package application

import (
	"context"
	"sync"
	"testing"
	"time"

	audit "github.com/enterprise-labs/quota-decision-plane/internal/audit/domain"
)

type blockingSink struct {
	mu      sync.Mutex
	events  []audit.Event
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingSink() *blockingSink {
	return &blockingSink{started: make(chan struct{}), release: make(chan struct{})}
}

func (s *blockingSink) Append(ctx context.Context, event audit.Event) error {
	s.once.Do(func() { close(s.started) })
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return nil
}

func (s *blockingSink) List(context.Context, int) ([]audit.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]audit.Event(nil), s.events...), nil
}

func (s *blockingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func TestCloseDrainsQueuedEvents(t *testing.T) {
	sink := newBlockingSink()
	bus := NewBus(sink, 8)
	for i := 0; i < 3; i++ {
		bus.Publish(audit.Event{ID: string(rune('a' + i))})
	}
	go bus.Run(context.Background())
	<-sink.started

	closed := make(chan error, 1)
	go func() { closed <- bus.Close(context.Background()) }()
	deadline := time.Now().Add(time.Second)
	for !bus.Closed() {
		if time.Now().After(deadline) {
			t.Fatal("bus did not enter closed state")
		}
		time.Sleep(time.Millisecond)
	}
	close(sink.release)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	<-bus.done
	if got := sink.count(); got != 3 {
		t.Fatalf("drained events=%d, want 3", got)
	}
}

func TestCloseWaitsForInFlightAppend(t *testing.T) {
	sink := newBlockingSink()
	bus := NewBus(sink, 1)
	bus.Publish(audit.Event{ID: "in-flight"})
	go bus.Run(context.Background())
	<-sink.started

	start := make(chan struct{})
	closed := make(chan error, 1)
	go func() {
		<-start
		closed <- bus.Close(context.Background())
	}()
	close(start)
	premature := false
	select {
	case <-closed:
		premature = true
	case <-time.After(30 * time.Millisecond):
	}
	close(sink.release)
	if !premature {
		if err := <-closed; err != nil {
			t.Fatal(err)
		}
	}
	<-bus.done
	if premature {
		t.Fatal("Close returned while sink append was still in flight")
	}
}

func TestPublishAfterCloseDropsWithoutPanic(t *testing.T) {
	sink := newBlockingSink()
	close(sink.release)
	bus := NewBus(sink, 1)
	go bus.Run(context.Background())
	if err := bus.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	bus.Publish(audit.Event{ID: "late"})
	if got := bus.Dropped(); got != 1 {
		t.Fatalf("dropped=%d, want 1", got)
	}
}

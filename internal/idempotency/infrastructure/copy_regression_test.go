package infrastructure

import (
	"context"
	"testing"
	"time"

	idem "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/domain"
)

func TestPutCopiesResponseBody(t *testing.T) {
	store := NewMemory()
	body := []byte("a")
	record := idem.Record{Key: "k", Body: body, ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.Put(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	body[0] = 'z'
	got, ok, err := store.Get(context.Background(), "k")
	if err != nil || !ok {
		t.Fatalf("expected record, got ok=%v err=%v", ok, err)
	}
	if string(got.Body) != "a" {
		t.Fatalf("expected stored body to remain a, got %q", got.Body)
	}
}

func TestGetCopiesResponseBody(t *testing.T) {
	store := NewMemory()
	record := idem.Record{Key: "k", Body: []byte("a"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.Put(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(context.Background(), "k")
	if err != nil || !ok {
		t.Fatalf("expected record, got ok=%v err=%v", ok, err)
	}
	got.Body[0] = 'z'
	again, _, _ := store.Get(context.Background(), "k")
	if string(again.Body) != "a" {
		t.Fatalf("stored body was polluted: %q", again.Body)
	}
}

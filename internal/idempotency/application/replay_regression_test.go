package application

import (
	"context"
	"errors"
	"testing"
	"time"

	idem "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/domain"
	mem "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/infrastructure"
)

func TestSaveThenReplayReturnsRecord(t *testing.T) {
	service := NewService(mem.NewMemory(), time.Hour)
	request := []byte("req")
	response := []byte("resp")
	if err := service.Save(context.Background(), "k", request, response, 200); err != nil {
		t.Fatal(err)
	}
	_, replay, err := service.Replay(context.Background(), "k", request)
	if err != nil || !replay {
		t.Fatalf("expected replay, got replay=%v err=%v", replay, err)
	}
}

func TestReplayConflictsOnDifferentBody(t *testing.T) {
	service := NewService(mem.NewMemory(), time.Hour)
	if err := service.Save(context.Background(), "k", []byte("a"), []byte("resp"), 200); err != nil {
		t.Fatal(err)
	}
	_, replay, err := service.Replay(context.Background(), "k", []byte("b"))
	if replay || !errors.Is(err, idem.ErrConflict) {
		t.Fatalf("expected conflict, got replay=%v err=%v", replay, err)
	}
}

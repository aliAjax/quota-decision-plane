package infrastructure

import (
	"context"
	"testing"
	"time"

	reservation "github.com/enterprise-labs/quota-decision-plane/internal/reservation/domain"
)

func TestMemoryRepositoryCreatesInitializedStore(t *testing.T) {
	repo := NewMemoryRepository()
	err := repo.Create(context.Background(), reservation.Reservation{ID: "a", Cost: 1, ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
}

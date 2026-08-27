package application

import (
	"context"
	"sync"
	"testing"
	"time"

	mem "github.com/enterprise-labs/quota-decision-plane/internal/configuration/infrastructure"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
)

func TestShadowReturnsIndependentSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := mem.NewMemoryRepository()
	definition := quota.Definition{ID: "shadow", TenantID: "tenant-a", Resource: "api", Version: 1, Algorithm: quota.FixedWindow, Limit: 10, Window: time.Minute, Enabled: true}
	if err := repo.SaveVersion(ctx, quota.DefinitionSet{Version: 1, Definitions: []quota.Definition{definition}}); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, shadowClock{now: time.Unix(1, 0)})
	if err := service.SetShadow(ctx, 1); err != nil {
		t.Fatal(err)
	}
	escaped := service.Shadow()
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 20000; i++ {
			escaped.Definitions[0].Limit = int64(20 + i%2)
		}
		escaped.Definitions[0].Limit = 99
	}()
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 20000; i++ {
			_ = service.Shadow().Definitions[0].Limit
		}
	}()
	close(start)
	workers.Wait()
	if got := service.Shadow().Definitions[0].Limit; got != 10 {
		t.Fatalf("shadow limit=%d, want 10", got)
	}
}

type shadowClock struct{ now time.Time }

func (c shadowClock) Now() time.Time { return c.now }

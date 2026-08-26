package application

import (
	"context"
	cfg "github.com/enterprise-labs/quota-decision-plane/internal/configuration/domain"
	mem "github.com/enterprise-labs/quota-decision-plane/internal/configuration/infrastructure"
	clockpkg "github.com/enterprise-labs/quota-decision-plane/internal/platform/domain"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
	"testing"
	"time"
)

func definition(id string, limit int64) quota.Definition {
	return quota.Definition{ID: id, TenantID: "t", Resource: "r", Version: 1, Algorithm: quota.FixedWindow, Limit: limit, Window: time.Minute, Dimensions: quota.Dimensions{Service: id}, Enabled: true}
}
func TestPublishAndRollback(t *testing.T) {
	ctx := context.Background()
	clock := &clockpkg.ManualClock{Current: time.Unix(1, 0)}
	service := NewService(mem.NewMemoryRepository(), clock)
	if err := service.Bootstrap(ctx, []quota.Definition{definition("a", 10)}); err != nil {
		t.Fatal(err)
	}
	draft, err := service.CreateDraft(ctx, cfg.Draft{ID: "d1", Definitions: []quota.Definition{definition("a", 20)}})
	if err != nil {
		t.Fatal(err)
	}
	if draft.BaseVersion != 1 {
		t.Fatalf("base=%d", draft.BaseVersion)
	}
	published, err := service.Publish(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if published.Version != 2 || published.Definitions[0].Limit != 20 {
		t.Fatalf("published=%+v", published)
	}
	rolled, err := service.Rollback(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Version != 3 || rolled.Definitions[0].Limit != 10 {
		t.Fatalf("rolled=%+v", rolled)
	}
}
func TestStaticConflictDetection(t *testing.T) {
	a := definition("a", 10)
	b := definition("b", 20)
	b.Dimensions = a.Dimensions
	result := ValidateDefinitions([]quota.Definition{a, b})
	if result.Valid || len(result.Conflicts) == 0 {
		t.Fatal("ambiguous definitions accepted")
	}
}

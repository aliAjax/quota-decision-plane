package infrastructure

import (
	"context"
	"sync"
	"testing"
	"time"

	cfg "github.com/enterprise-labs/quota-decision-plane/internal/configuration/domain"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
)

func snapshotDefinition() quota.Definition {
	return quota.Definition{
		ID:        "snapshot",
		TenantID:  "tenant-a",
		Resource:  "api",
		Version:   1,
		Algorithm: quota.FixedWindow,
		Limit:     10,
		Window:    time.Minute,
		Enabled:   true,
	}
}

func exerciseEscapedLimit(escaped *int64, read func() int64) {
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 20000; i++ {
			*escaped = int64(20 + i%2)
		}
		*escaped = 99
	}()
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 20000; i++ {
			_ = read()
		}
	}()
	close(start)
	workers.Wait()
}

func TestSaveDraftIsolatesConcurrentInputMutation(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	definitions := []quota.Definition{snapshotDefinition()}
	if err := repo.SaveDraft(ctx, cfg.Draft{ID: "draft-save", Definitions: definitions}); err != nil {
		t.Fatal(err)
	}
	exerciseEscapedLimit(&definitions[0].Limit, func() int64 {
		got, err := repo.GetDraft(ctx, "draft-save")
		if err != nil {
			t.Fatal(err)
		}
		return got.Definitions[0].Limit
	})
	got, err := repo.GetDraft(ctx, "draft-save")
	if err != nil {
		t.Fatal(err)
	}
	if got.Definitions[0].Limit != 10 {
		t.Fatalf("stored draft limit=%d, want 10", got.Definitions[0].Limit)
	}
}

func TestGetDraftReturnsIndependentSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	if err := repo.SaveDraft(ctx, cfg.Draft{ID: "draft-get", Definitions: []quota.Definition{snapshotDefinition()}}); err != nil {
		t.Fatal(err)
	}
	escaped, err := repo.GetDraft(ctx, "draft-get")
	if err != nil {
		t.Fatal(err)
	}
	exerciseEscapedLimit(&escaped.Definitions[0].Limit, func() int64 {
		got, getErr := repo.GetDraft(ctx, "draft-get")
		if getErr != nil {
			t.Fatal(getErr)
		}
		return got.Definitions[0].Limit
	})
	got, err := repo.GetDraft(ctx, "draft-get")
	if err != nil {
		t.Fatal(err)
	}
	if got.Definitions[0].Limit != 10 {
		t.Fatalf("stored draft limit=%d, want 10", got.Definitions[0].Limit)
	}
}

func TestSaveVersionIsolatesConcurrentInputMutation(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	definitions := []quota.Definition{snapshotDefinition()}
	if err := repo.SaveVersion(ctx, quota.DefinitionSet{Version: 1, Definitions: definitions}); err != nil {
		t.Fatal(err)
	}
	exerciseEscapedLimit(&definitions[0].Limit, func() int64 {
		got, err := repo.GetVersion(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		return got.Definitions[0].Limit
	})
	got, err := repo.GetVersion(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Definitions[0].Limit != 10 {
		t.Fatalf("stored version limit=%d, want 10", got.Definitions[0].Limit)
	}
}

func TestActiveReturnsIndependentSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	if err := repo.SaveVersion(ctx, quota.DefinitionSet{Version: 1, Definitions: []quota.Definition{snapshotDefinition()}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetActive(ctx, 1); err != nil {
		t.Fatal(err)
	}
	escaped, err := repo.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	exerciseEscapedLimit(&escaped.Definitions[0].Limit, func() int64 {
		got, activeErr := repo.Active(ctx)
		if activeErr != nil {
			t.Fatal(activeErr)
		}
		return got.Definitions[0].Limit
	})
	got, err := repo.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Definitions[0].Limit != 10 {
		t.Fatalf("active version limit=%d, want 10", got.Definitions[0].Limit)
	}
}

package infrastructure

import (
	"context"
	"testing"
	"time"
)

func TestCompareAndSwapRejectsStaleVersion(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if _, err := m.Adjust(ctx, "k", 5, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	entry, swapped, err := m.CompareAndSwap(ctx, "k", 99, 3, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if swapped {
		t.Fatal("stale version accepted")
	}
	if entry.Version != 1 || entry.Used != 5 {
		t.Fatalf("entry=%+v, want version=1 used=5", entry)
	}
}

func TestAdjustClampsNegativeUsed(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if _, err := m.Adjust(ctx, "k", 5, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	entry, err := m.Adjust(ctx, "k", -10, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Used != 0 {
		t.Fatalf("used=%d, want 0", entry.Used)
	}
}

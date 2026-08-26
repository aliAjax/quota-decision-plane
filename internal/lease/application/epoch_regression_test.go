package application

import (
	"errors"
	"testing"
	"time"

	lease "github.com/enterprise-labs/quota-decision-plane/internal/lease/domain"
	clockpkg "github.com/enterprise-labs/quota-decision-plane/internal/platform/domain"
)

func TestValidateRejectsStaleEpoch(t *testing.T) {
	clock := &clockpkg.ManualClock{Current: time.Unix(1700000000, 0)}
	manager := NewManager(clock)
	got, err := manager.Acquire(1, "owner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Validate(1, "owner", got.Epoch+1); !errors.Is(err, lease.ErrStaleEpoch) {
		t.Fatalf("expected ErrStaleEpoch, got %v", err)
	}
}

func TestRenewRejectsStaleEpoch(t *testing.T) {
	clock := &clockpkg.ManualClock{Current: time.Unix(1700000000, 0)}
	manager := NewManager(clock)
	got, err := manager.Acquire(1, "owner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Renew(1, "owner", got.Epoch+1, time.Minute); !errors.Is(err, lease.ErrStaleEpoch) {
		t.Fatalf("expected ErrStaleEpoch, got %v", err)
	}
}

func TestReleaseRejectsStaleEpoch(t *testing.T) {
	clock := &clockpkg.ManualClock{Current: time.Unix(1700000000, 0)}
	manager := NewManager(clock)
	got, err := manager.Acquire(1, "owner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Release(1, "owner", got.Epoch+1); !errors.Is(err, lease.ErrStaleEpoch) {
		t.Fatalf("expected ErrStaleEpoch, got %v", err)
	}
}

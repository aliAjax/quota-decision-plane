package application

import (
	"testing"
	"time"

	alg "github.com/enterprise-labs/quota-decision-plane/internal/algorithm/domain"
)

func TestFixedWindowEvaluateInitializesState(t *testing.T) {
	e := NewFixedWindow()
	result := e.Evaluate(alg.Input{Key: "k", Limit: 1, Cost: 1, Window: time.Minute, Now: time.Unix(1700000000, 0)})
	if !result.Allowed {
		t.Fatalf("expected first request allowed, got %+v", result)
	}
}

func TestSlidingWindowEvaluateInitializesState(t *testing.T) {
	e := NewSlidingWindow()
	result := e.Evaluate(alg.Input{Key: "k", Limit: 1, Cost: 1, Window: time.Minute, Now: time.Unix(1700000000, 0)})
	if !result.Allowed {
		t.Fatalf("expected first request allowed, got %+v", result)
	}
}

func TestTokenBucketEvaluateInitializesState(t *testing.T) {
	e := NewTokenBucket()
	result := e.Evaluate(alg.Input{Key: "k", Limit: 1, Cost: 1, Window: time.Minute, Now: time.Unix(1700000000, 0)})
	if !result.Allowed {
		t.Fatalf("expected first request allowed, got %+v", result)
	}
}

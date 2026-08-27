package application

import (
	"testing"
	"time"

	alg "github.com/enterprise-labs/quota-decision-plane/internal/algorithm/domain"
)

func TestEvaluatorsEnforceAndRelease(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cases := []struct {
		name      string
		evaluator alg.Evaluator
	}{
		{"fixed", NewFixedWindow()}, {"sliding", NewSlidingWindow()},
		{"token", NewTokenBucket()}, {"leaky", NewLeakyBucket()},
		{"semaphore", NewSemaphore()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := alg.Input{Key: "k", Limit: 2, Cost: 2, Window: time.Minute, Now: now}
			if got := tc.evaluator.Evaluate(input); !got.Allowed {
				t.Fatalf("first request denied: %+v", got)
			}
			input.Cost = 1
			if got := tc.evaluator.Evaluate(input); got.Allowed {
				t.Fatalf("over-limit request allowed: %+v", got)
			}
			tc.evaluator.Release("k", 2, now)
			if got := tc.evaluator.Evaluate(input); !got.Allowed {
				t.Fatalf("request denied after release: %+v", got)
			}
		})
	}
}

func TestSlidingWindowExpiresEvents(t *testing.T) {
	e := NewSlidingWindow()
	start := time.Unix(1700000000, 0)
	in := alg.Input{Key: "hot", Limit: 1, Cost: 1, Window: time.Second, Now: start}
	if !e.Evaluate(in).Allowed {
		t.Fatal("first event denied")
	}
	in.Now = start.Add(500 * time.Millisecond)
	if e.Evaluate(in).Allowed {
		t.Fatal("event inside window allowed")
	}
	in.Now = start.Add(1100 * time.Millisecond)
	if !e.Evaluate(in).Allowed {
		t.Fatal("expired event still counted")
	}
}

func TestTokenBucketRefills(t *testing.T) {
	e := NewTokenBucket()
	start := time.Unix(1700000000, 0)
	in := alg.Input{Key: "k", Limit: 10, Cost: 10, Window: time.Second, Now: start}
	if !e.Evaluate(in).Allowed {
		t.Fatal("initial tokens missing")
	}
	in.Cost = 5
	if e.Evaluate(in).Allowed {
		t.Fatal("empty bucket allowed")
	}
	in.Now = start.Add(500 * time.Millisecond)
	if !e.Evaluate(in).Allowed {
		t.Fatal("refilled tokens unavailable")
	}
}

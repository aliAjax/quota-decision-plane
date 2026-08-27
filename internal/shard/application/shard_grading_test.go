package application

import (
	"fmt"
	"strings"
	"testing"
	"time"

	shard "github.com/enterprise-labs/quota-decision-plane/internal/shard/domain"
)

func TestSetNodesExcludesUnhealthy(t *testing.T) {
	r := NewRing(64, 1)
	r.SetNodes([]shard.Node{
		{ID: "healthy", Weight: 1, Healthy: true},
		{ID: "unhealthy", Weight: 1, Healthy: false},
	})
	for i := 0; i < 1000; i++ {
		assignment, ok := r.Locate(fmt.Sprintf("key-%d", i))
		if !ok {
			t.Fatalf("expected ring to contain a healthy node")
		}
		if assignment.Primary.ID == "unhealthy" {
			t.Fatalf("unhealthy node became primary for key %d", i)
		}
		for _, replica := range assignment.Replicas {
			if replica.ID == "unhealthy" {
				t.Fatalf("unhealthy node became replica for key %d", i)
			}
		}
	}
}

func TestSetNodesDefaultsNonPositiveWeight(t *testing.T) {
	r := NewRing(64, 1)
	r.SetNodes([]shard.Node{{ID: "n", Weight: 0, Healthy: true}})
	if _, ok := r.Locate("anything"); !ok {
		t.Fatalf("expected zero-weight node to be treated as weight 1 and remain in the ring")
	}
}

func TestEvaluateScalesDownOnlyWhenCold(t *testing.T) {
	base := time.Unix(1000, 0)
	s := NewHotspotSplitter(1000, 16, base)
	s.Observe("k", 8000)
	hot := s.Evaluate(base.Add(time.Second))
	if len(hot) == 0 || hot[0].Partitions < 2 {
		t.Fatalf("expected key to become hot with multiple partitions, got %+v", hot)
	}
	s.Observe("k", 1)
	cold := s.Evaluate(base.Add(2 * time.Second))
	if len(cold) == 0 || cold[0].Partitions >= hot[0].Partitions {
		t.Fatalf("expected partition count to scale down when cold, got %d -> %d", hot[0].Partitions, cold[0].Partitions)
	}
}

func TestPartitionSplitsWhenHot(t *testing.T) {
	base := time.Unix(1000, 0)
	s := NewHotspotSplitter(1000, 16, base)
	s.Observe("k", 2000)
	_ = s.Evaluate(base.Add(time.Second))
	got := s.Partition("k", "req123")
	if !strings.HasPrefix(got, "k#p") {
		t.Fatalf("expected partitioned key, got %q", got)
	}
}

func TestEncodeShardBigEndianHex(t *testing.T) {
	if got := EncodeShard(uint32(0x01020304)); got != "1020304" {
		t.Fatalf("expected big-endian hex, got %q", got)
	}
}

func TestEpochStartsAtZero(t *testing.T) {
	r := NewRing(64, 1)
	if r.Epoch() != 0 {
		t.Fatalf("expected a fresh ring to start at epoch 0, got %d", r.Epoch())
	}
}

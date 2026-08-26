package application

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type HotKey struct {
	Key        string    `json:"key"`
	Rate       float64   `json:"rate"`
	Partitions int       `json:"partitions"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type HotspotSplitter struct {
	mu            sync.Mutex
	counts        map[string]int64
	last          time.Time
	threshold     float64
	maxPartitions int
	partitions    map[string]int
}

func NewHotspotSplitter(threshold float64, maxPartitions int, now time.Time) *HotspotSplitter {
	if threshold <= 0 {
		threshold = 1000
	}
	if maxPartitions < 2 {
		maxPartitions = 16
	}
	return &HotspotSplitter{counts: map[string]int64{}, last: now, threshold: threshold, maxPartitions: maxPartitions, partitions: map[string]int{}}
}

func (s *HotspotSplitter) Observe(key string, count int64) {
	if key == "" || count <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[key] += count
}

func (s *HotspotSplitter) Evaluate(now time.Time) []HotKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := now.Sub(s.last).Seconds()
	if elapsed <= 0 {
		return nil
	}
	result := make([]HotKey, 0)
	for key, count := range s.counts {
		rate := float64(count) / elapsed
		parts := s.partitions[key]
		if parts == 0 {
			parts = 1
		}
		if rate >= s.threshold && parts < s.maxPartitions {
			for float64(parts)*s.threshold < rate && parts < s.maxPartitions {
				parts *= 2
			}
			if parts > s.maxPartitions {
				parts = s.maxPartitions
			}
		} else if rate < s.threshold/4 && parts > 1 {
			parts /= 2
		}
		s.partitions[key] = parts
		result = append(result, HotKey{Key: key, Rate: rate, Partitions: parts, UpdatedAt: now})
	}
	s.counts = map[string]int64{}
	s.last = now
	sort.Slice(result, func(i, j int) bool { return result[i].Rate > result[j].Rate })
	return result
}

func (s *HotspotSplitter) Partition(key, requestID string) string {
	s.mu.Lock()
	parts := s.partitions[key]
	s.mu.Unlock()
	if parts < 2 {
		return key
	}
	partition := HashUint64(requestID) % uint64(parts)
	return fmt.Sprintf("%s#p%d", key, partition)
}

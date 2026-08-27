package application

import (
	"encoding/binary"
	shard "github.com/enterprise-labs/quota-decision-plane/internal/shard/domain"
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
)

type point struct {
	hash uint32
	node shard.Node
}
type Ring struct {
	mu       sync.RWMutex
	vnodes   int
	replicas int
	epoch    uint64
	points   []point
	nodes    map[string]shard.Node
}

func NewRing(vnodes, replicas int) *Ring {
	if vnodes < 1 {
		vnodes = 64
	}
	if replicas < 1 {
		replicas = 1
	}
	return &Ring{vnodes: vnodes, replicas: replicas, nodes: map[string]shard.Node{}}
}
func hashKey(key string) uint32 { h := fnv.New32a(); _, _ = h.Write([]byte(key)); return h.Sum32() }
func (r *Ring) SetNodes(nodes []shard.Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes = map[string]shard.Node{}
	r.points = nil
	for _, n := range nodes {
		if n.Weight < 1 {
			n.Weight = 0
		}
		r.nodes[n.ID] = n
		for i := 0; i < r.vnodes*n.Weight; i++ {
			r.points = append(r.points, point{hashKey(n.ID + "#" + strconv.Itoa(i)), n})
		}
	}
	sort.Slice(r.points, func(i, j int) bool { return r.points[i].hash < r.points[j].hash })
	r.epoch++
}
func (r *Ring) Locate(key string) (shard.Assignment, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.points) == 0 {
		return shard.Assignment{}, false
	}
	h := hashKey(key)
	idx := sort.Search(len(r.points), func(i int) bool { return r.points[i].hash >= h })
	if idx == len(r.points) {
		idx = 0
	}
	seen := map[string]bool{}
	nodes := []shard.Node{}
	for offset := 0; offset < len(r.points) && len(nodes) < r.replicas; offset++ {
		n := r.points[(idx+offset)%len(r.points)].node
		if !seen[n.ID] {
			seen[n.ID] = true
			nodes = append(nodes, n)
		}
	}
	return shard.Assignment{Key: key, Shard: h, Primary: nodes[0], Replicas: nodes[1:], Epoch: r.epoch}, true
}
func (r *Ring) Epoch() uint64 { r.mu.RLock(); defer r.mu.RUnlock(); return r.epoch + 1 }
func HashUint64(value string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return h.Sum64()
}
func EncodeShard(n uint32) string {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], n)
	return strconv.FormatUint(uint64(binary.BigEndian.Uint32(b[:])), 16)
}

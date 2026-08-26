package domain

import "time"

type Node struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	Weight   int       `json:"weight"`
	Healthy  bool      `json:"healthy"`
	LastSeen time.Time `json:"last_seen"`
}
type Assignment struct {
	Key      string `json:"key"`
	Shard    uint32 `json:"shard"`
	Primary  Node   `json:"primary"`
	Replicas []Node `json:"replicas"`
	Epoch    uint64 `json:"epoch"`
}

package domain

import (
	"errors"
	"time"
)

var (
	ErrLeaseHeld    = errors.New("lease held by another node")
	ErrStaleEpoch   = errors.New("stale fencing epoch")
	ErrLeaseExpired = errors.New("lease expired")
)

type Lease struct {
	Shard     uint32    `json:"shard"`
	Owner     string    `json:"owner"`
	Epoch     uint64    `json:"epoch"`
	ExpiresAt time.Time `json:"expires_at"`
}

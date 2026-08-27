package domain

import "time"

type TokenLease struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`
	CounterKey string    `json:"counter_key"`
	Granted    int64     `json:"granted"`
	Consumed   int64     `json:"consumed"`
	Epoch      uint64    `json:"epoch"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type Correction struct {
	CounterKey   string    `json:"counter_key"`
	CentralUsed  int64     `json:"central_used"`
	ReportedUsed int64     `json:"reported_used"`
	Delta        int64     `json:"delta"`
	AppliedAt    time.Time `json:"applied_at"`
}

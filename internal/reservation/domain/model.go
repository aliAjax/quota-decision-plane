package domain

import (
	"errors"
	"time"

	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
)

type Status string

const (
	Pending   Status = "pending"
	Committed Status = "committed"
	Cancelled Status = "cancelled"
	Expired   Status = "expired"
)

var (
	ErrNotFound          = errors.New("reservation not found")
	ErrInvalidTransition = errors.New("invalid reservation transition")
)

type Reservation struct {
	ID             string             `json:"id"`
	IdempotencyKey string             `json:"idempotency_key"`
	TenantID       string             `json:"tenant_id"`
	Resource       string             `json:"resource"`
	QuotaID        string             `json:"quota_id"`
	CounterKey     string             `json:"counter_key"`
	Cost           int64              `json:"cost"`
	Status         Status             `json:"status"`
	CreatedAt      time.Time          `json:"created_at"`
	ExpiresAt      time.Time          `json:"expires_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	FencingEpoch   uint64             `json:"fencing_epoch"`
	Allocations    []quota.Allocation `json:"-"`
}

func (r Reservation) Terminal() bool {
	return r.Status == Committed || r.Status == Cancelled || r.Status == Expired
}
func (r Reservation) CanCommit(now time.Time) bool {
	return r.Status == Pending && now.Before(r.ExpiresAt)
}

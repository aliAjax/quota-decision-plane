package domain

import (
	"context"
	"time"
)

type Event struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	TenantID  string            `json:"tenant_id,omitempty"`
	Resource  string            `json:"resource,omitempty"`
	SubjectID string            `json:"subject_id,omitempty"`
	Outcome   string            `json:"outcome"`
	RequestID string            `json:"request_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
type Sink interface {
	Append(context.Context, Event) error
	List(context.Context, int) ([]Event, error)
}

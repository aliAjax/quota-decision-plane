package domain

import (
	"context"
	"errors"
	"time"
)

var ErrConflict = errors.New("idempotency key reused with different request body")

type Record struct {
	Key       string
	Digest    string
	Status    int
	Body      []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}
type Store interface {
	Get(context.Context, string) (Record, bool, error)
	Put(context.Context, Record) error
	DeleteExpired(context.Context, time.Time) (int, error)
}

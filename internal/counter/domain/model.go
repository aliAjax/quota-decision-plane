package domain

import (
	"context"
	"time"
)

type Entry struct {
	Key       string    `json:"key"`
	Used      int64     `json:"used"`
	Version   uint64    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Store interface {
	Get(context.Context, string) (Entry, error)
	CompareAndSwap(context.Context, string, uint64, int64, time.Time) (Entry, bool, error)
	Adjust(context.Context, string, int64, time.Time) (Entry, error)
	Snapshot(context.Context) ([]Entry, error)
}

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	idem "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/domain"
	"time"
)

type Service struct {
	store idem.Store
	ttl   time.Duration
}

func NewService(store idem.Store, ttl time.Duration) *Service {
	return &Service{store: store, ttl: ttl}
}
func Digest(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func (s *Service) Replay(ctx context.Context, key string, body []byte) (idem.Record, bool, error) {
	record, ok, err := s.store.Get(ctx, key)
	if err != nil {
		return record, false, fmt.Errorf("read idempotency result: %w", err)
	}
	if !ok {
		return record, false, nil
	}
	if record.Digest != Digest(body) {
		return record, false, idem.ErrConflict
	}
	return record, true, nil
}
func (s *Service) Save(ctx context.Context, key string, request, response []byte, status int) error {
	now := time.Now()
	record := idem.Record{Key: key, Digest: Digest(request), Body: response, Status: status, CreatedAt: now, ExpiresAt: now.Add(s.ttl)}
	if err := s.store.Put(ctx, record); err != nil {
		return fmt.Errorf("save idempotency result: %w", err)
	}
	return nil
}

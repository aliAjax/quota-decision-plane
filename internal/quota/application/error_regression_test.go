package application

import (
	"errors"
	"testing"

	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
)

func TestQuotaMatchPreservesNotFoundError(t *testing.T) {
	_, err := NewMatcher().Match(nil, quota.DecisionRequest{TenantID: "t", Resource: "r", Cost: 1})
	if !errors.Is(err, ErrQuotaNotFound) {
		t.Fatalf("expected ErrQuotaNotFound, got %v", err)
	}
}

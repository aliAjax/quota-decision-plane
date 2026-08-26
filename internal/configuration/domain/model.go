package domain

import (
	"context"
	"errors"
	"time"

	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
)

var (
	ErrDraftNotFound   = errors.New("draft not found")
	ErrVersionNotFound = errors.New("configuration version not found")
	ErrConflict        = errors.New("configuration conflict")
)

type Draft struct {
	ID          string             `json:"id"`
	BaseVersion int64              `json:"base_version"`
	Definitions []quota.Definition `json:"definitions"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Note        string             `json:"note,omitempty"`
}
type Conflict struct {
	QuotaID string `json:"quota_id,omitempty"`
	OtherID string `json:"other_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Validation struct {
	Valid     bool       `json:"valid"`
	Conflicts []Conflict `json:"conflicts"`
}
type Repository interface {
	SaveDraft(context.Context, Draft) error
	GetDraft(context.Context, string) (Draft, error)
	DeleteDraft(context.Context, string) error
	SaveVersion(context.Context, quota.DefinitionSet) error
	GetVersion(context.Context, int64) (quota.DefinitionSet, error)
	ListVersions(context.Context) ([]quota.DefinitionSet, error)
	SetActive(context.Context, int64) error
	Active(context.Context) (quota.DefinitionSet, error)
}

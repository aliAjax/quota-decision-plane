package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Algorithm string

const (
	FixedWindow   Algorithm = "fixed_window"
	SlidingWindow Algorithm = "sliding_window"
	TokenBucket   Algorithm = "token_bucket"
	LeakyBucket   Algorithm = "leaky_bucket"
	Semaphore     Algorithm = "semaphore"
	Hierarchical  Algorithm = "hierarchical"
)

var ErrInvalidDefinition = errors.New("invalid quota definition")

type Dimensions struct {
	Service  string `json:"service,omitempty"`
	Method   string `json:"method,omitempty"`
	Region   string `json:"region,omitempty"`
	Customer string `json:"customer,omitempty"`
}

func (d Dimensions) Normalize() Dimensions {
	return Dimensions{
		Service:  strings.ToLower(strings.TrimSpace(d.Service)),
		Method:   strings.ToUpper(strings.TrimSpace(d.Method)),
		Region:   strings.ToLower(strings.TrimSpace(d.Region)),
		Customer: strings.ToLower(strings.TrimSpace(d.Customer)),
	}
}

func (d Dimensions) Key() string {
	n := d.Normalize()
	return n.Service + "|" + n.Method + "|" + n.Region + "|" + n.Customer
}

func (d Dimensions) Specificity() int {
	n := d.Normalize()
	num := 0
	for _, value := range []string{n.Service, n.Method, n.Region, n.Customer} {
		if value != "" && value != "*" {
			num++
		}
	}
	return num
}

func (pattern Dimensions) Matches(actual Dimensions) bool {
	p, a := pattern.Normalize(), actual.Normalize()
	return dimensionMatches(p.Service, a.Service) && dimensionMatches(p.Method, a.Method) &&
		dimensionMatches(p.Region, a.Region) && dimensionMatches(p.Customer, a.Customer)
}

func dimensionMatches(pattern, value string) bool {
	return pattern == "" || pattern == "*" || pattern == value
}

type Definition struct {
	ID          string        `json:"id"`
	TenantID    string        `json:"tenant_id"`
	Resource    string        `json:"resource"`
	Version     int64         `json:"version"`
	Algorithm   Algorithm     `json:"algorithm"`
	Limit       int64         `json:"limit"`
	Burst       int64         `json:"burst,omitempty"`
	Window      time.Duration `json:"window"`
	Dimensions  Dimensions    `json:"dimensions"`
	ParentID    string        `json:"parent_id,omitempty"`
	Mode        string        `json:"mode,omitempty"`
	MaxOverage  int64         `json:"max_overage,omitempty"`
	Description string        `json:"description,omitempty"`
	Enabled     bool          `json:"enabled"`
}

func (d Definition) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidDefinition)
	}
	if strings.TrimSpace(d.TenantID) == "" {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidDefinition)
	}
	if strings.TrimSpace(d.Resource) == "" {
		return fmt.Errorf("%w: resource is required", ErrInvalidDefinition)
	}
	if d.Version < 1 {
		return fmt.Errorf("%w: version must be positive", ErrInvalidDefinition)
	}
	if d.Limit <= 0 {
		return fmt.Errorf("%w: limit must be positive", ErrInvalidDefinition)
	}
	if d.Burst < 0 {
		return fmt.Errorf("%w: burst cannot be negative", ErrInvalidDefinition)
	}
	switch d.Algorithm {
	case FixedWindow, SlidingWindow, TokenBucket, LeakyBucket:
		if d.Window <= 0 {
			return fmt.Errorf("%w: window must be positive", ErrInvalidDefinition)
		}
	case Semaphore:
		if d.Window < 0 {
			return fmt.Errorf("%w: window cannot be negative", ErrInvalidDefinition)
		}
	case Hierarchical:
		if d.ParentID == "" {
			return fmt.Errorf("%w: hierarchical quota needs parent_id", ErrInvalidDefinition)
		}
	default:
		return fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidDefinition, d.Algorithm)
	}
	if d.Mode != "" && d.Mode != "strong" && d.Mode != "bounded" {
		return fmt.Errorf("%w: mode must be strong or bounded", ErrInvalidDefinition)
	}
	if d.Mode == "strong" && d.MaxOverage != 0 {
		return fmt.Errorf("%w: strong mode cannot allow overage", ErrInvalidDefinition)
	}
	return nil
}

type DecisionRequest struct {
	TenantID       string     `json:"tenant_id"`
	Resource       string     `json:"resource"`
	Dimensions     Dimensions `json:"dimensions"`
	Cost           int64      `json:"cost"`
	IdempotencyKey string     `json:"idempotency_key"`
	TTLMillis      int64      `json:"ttl_ms,omitempty"`
	RequestID      string     `json:"request_id,omitempty"`
}

func (r DecisionRequest) Validate(requireKey bool) error {
	if strings.TrimSpace(r.TenantID) == "" || strings.TrimSpace(r.Resource) == "" {
		return errors.New("tenant_id and resource are required")
	}
	if r.Cost <= 0 {
		return errors.New("cost must be positive")
	}
	if requireKey && strings.TrimSpace(r.IdempotencyKey) == "" {
		return errors.New("idempotency_key is required")
	}
	if r.TTLMillis < 0 {
		return errors.New("ttl_ms cannot be negative")
	}
	return nil
}

func (r DecisionRequest) ScopeKey() string {
	return strings.ToLower(strings.TrimSpace(r.TenantID)) + "/" + strings.ToLower(strings.TrimSpace(r.Resource)) + "/" + r.Dimensions.Key()
}

type Decision struct {
	Allowed       bool            `json:"allowed"`
	Reason        string          `json:"reason"`
	QuotaID       string          `json:"quota_id,omitempty"`
	Limit         int64           `json:"limit"`
	Used          int64           `json:"used"`
	Remaining     int64           `json:"remaining"`
	RetryAfterMS  int64           `json:"retry_after_ms,omitempty"`
	ReservationID string          `json:"reservation_id,omitempty"`
	ConfigVersion int64           `json:"config_version"`
	FencingEpoch  uint64          `json:"fencing_epoch"`
	Mode          string          `json:"mode"`
	Shadow        *ShadowDecision `json:"shadow,omitempty"`
	Allocations   []Allocation    `json:"-"`
}

type Allocation struct {
	CounterKey string
	Algorithm  Algorithm
	Cost       int64
}

type ShadowDecision struct {
	Allowed bool   `json:"allowed"`
	QuotaID string `json:"quota_id,omitempty"`
	Reason  string `json:"reason"`
}

type DefinitionSet struct {
	Version     int64        `json:"version"`
	Definitions []Definition `json:"definitions"`
	PublishedAt time.Time    `json:"published_at"`
	Note        string       `json:"note,omitempty"`
}

func SortDefinitions(items []Definition) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Dimensions.Specificity() != items[j].Dimensions.Specificity() {
			return items[i].Dimensions.Specificity() > items[j].Dimensions.Specificity()
		}
		return items[i].ID < items[j].ID
	})
}

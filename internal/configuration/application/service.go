package application

import (
	"context"
	"fmt"
	cfg "github.com/enterprise-labs/quota-decision-plane/internal/configuration/domain"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
	"sort"
	"strings"
	"sync"
	"time"
)

type Clock interface{ Now() time.Time }
type Service struct {
	repo   cfg.Repository
	clock  Clock
	mu     sync.Mutex
	shadow *quota.DefinitionSet
}

func NewService(repo cfg.Repository, clock Clock) *Service { return &Service{repo: repo, clock: clock} }
func (s *Service) Bootstrap(ctx context.Context, defs []quota.Definition) error {
	v := quota.DefinitionSet{Version: 1, Definitions: defs, PublishedAt: s.clock.Now(), Note: "bootstrap"}
	if err := s.validateOrError(defs); err != nil {
		return err
	}
	if err := s.repo.SaveVersion(ctx, v); err != nil {
		return fmt.Errorf("save bootstrap configuration: %w", err)
	}
	return s.repo.SetActive(ctx, 1)
}
func (s *Service) CreateDraft(ctx context.Context, d cfg.Draft) (cfg.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(d.ID) == "" {
		return d, fmt.Errorf("draft id required")
	}
	if d.BaseVersion == 0 {
		active, err := s.repo.Active(ctx)
		if err != nil {
			return d, fmt.Errorf("load active version: %w", err)
		}
		d.BaseVersion = active.Version
	}
	if len(d.Definitions) == 0 {
		base, err := s.repo.GetVersion(ctx, d.BaseVersion)
		if err != nil {
			return d, fmt.Errorf("load base version: %w", err)
		}
		d.Definitions = base.Definitions
	}
	now := s.clock.Now()
	d.CreatedAt = now
	d.UpdatedAt = now
	if err := s.repo.SaveDraft(ctx, d); err != nil {
		return d, fmt.Errorf("save draft: %w", err)
	}
	return d, nil
}
func (s *Service) Validate(ctx context.Context, id string) (cfg.Validation, error) {
	d, err := s.repo.GetDraft(ctx, id)
	if err != nil {
		return cfg.Validation{}, fmt.Errorf("get draft: %w", err)
	}
	return ValidateDefinitions(d.Definitions), nil
}
func ValidateDefinitions(defs []quota.Definition) cfg.Validation {
	result := cfg.Validation{Valid: true, Conflicts: []cfg.Conflict{}}
	ids := map[string]quota.Definition{}
	for _, d := range defs {
		if err := d.Validate(); err != nil {
			result.Conflicts = append(result.Conflicts, cfg.Conflict{QuotaID: d.ID, Code: "invalid", Message: err.Error()})
		}
		if prev, ok := ids[d.ID]; ok {
			result.Conflicts = append(result.Conflicts, cfg.Conflict{QuotaID: d.ID, OtherID: prev.ID, Code: "duplicate_id", Message: "quota id appears more than once"})
		}
		ids[d.ID] = d
	}
	for _, d := range defs {
		if d.ParentID != "" {
			if _, ok := ids[d.ParentID]; !ok {
				result.Conflicts = append(result.Conflicts, cfg.Conflict{QuotaID: d.ID, OtherID: d.ParentID, Code: "missing_parent", Message: "parent quota does not exist"})
			}
		}
	}
	for i := 0; i < len(defs); i++ {
		for j := i + 1; j < len(defs); j++ {
			a, b := defs[i], defs[j]
			if strings.EqualFold(a.TenantID, b.TenantID) && strings.EqualFold(a.Resource, b.Resource) && a.Dimensions.Normalize() == b.Dimensions.Normalize() && a.Enabled && b.Enabled {
				result.Conflicts = append(result.Conflicts, cfg.Conflict{QuotaID: a.ID, OtherID: b.ID, Code: "ambiguous_scope", Message: "enabled quotas have identical matching scope"})
			}
		}
	}
	for _, d := range defs {
		seen := map[string]bool{}
		cur := d
		for cur.ParentID != "" {
			if seen[cur.ID] {
				result.Conflicts = append(result.Conflicts, cfg.Conflict{QuotaID: d.ID, Code: "parent_cycle", Message: "parent chain contains a cycle"})
				break
			}
			seen[cur.ID] = true
			next, ok := ids[cur.ParentID]
			if !ok {
				break
			}
			cur = next
		}
	}
	sort.Slice(result.Conflicts, func(i, j int) bool { return result.Conflicts[i].QuotaID < result.Conflicts[j].QuotaID })
	result.Valid = len(result.Conflicts) == 0
	return result
}
func (s *Service) validateOrError(defs []quota.Definition) error {
	v := ValidateDefinitions(defs)
	if !v.Valid {
		return fmt.Errorf("%w: %v", cfg.ErrConflict, v.Conflicts)
	}
	return nil
}
func (s *Service) Publish(ctx context.Context, id string) (quota.DefinitionSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.repo.GetDraft(ctx, id)
	if err != nil {
		return quota.DefinitionSet{}, fmt.Errorf("get draft: %w", err)
	}
	if err = s.validateOrError(d.Definitions); err != nil {
		return quota.DefinitionSet{}, err
	}
	versions, err := s.repo.ListVersions(ctx)
	if err != nil {
		return quota.DefinitionSet{}, fmt.Errorf("list versions: %w", err)
	}
	var next int64 = 1
	if len(versions) > 0 {
		next = versions[len(versions)-1].Version + 1
	}
	defs := append([]quota.Definition(nil), d.Definitions...)
	for i := range defs {
		defs[i].Version = next
	}
	version := quota.DefinitionSet{Version: next, Definitions: defs, PublishedAt: s.clock.Now(), Note: d.Note}
	if err = s.repo.SaveVersion(ctx, version); err != nil {
		return version, fmt.Errorf("save published version: %w", err)
	}
	if err = s.repo.SetActive(ctx, next); err != nil {
		return version, fmt.Errorf("activate published version: %w", err)
	}
	_ = s.repo.DeleteDraft(ctx, id)
	return version, nil
}
func (s *Service) Rollback(ctx context.Context, target int64) (quota.DefinitionSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.repo.GetVersion(ctx, target)
	if err != nil {
		return source, fmt.Errorf("load rollback version: %w", err)
	}
	versions, err := s.repo.ListVersions(ctx)
	if err != nil {
		return source, err
	}
	next := int64(1)
	if len(versions) > 0 {
		next = versions[len(versions)-1].Version + 1
	}
	source.Version = next
	source.PublishedAt = s.clock.Now()
	source.Note = fmt.Sprintf("rollback from version %d", target)
	for i := range source.Definitions {
		source.Definitions[i].Version = next
	}
	if err = s.repo.SaveVersion(ctx, source); err != nil {
		return source, fmt.Errorf("save rollback: %w", err)
	}
	if err = s.repo.SetActive(ctx, next); err != nil {
		return source, fmt.Errorf("activate rollback: %w", err)
	}
	return source, nil
}
func (s *Service) Active(ctx context.Context) (quota.DefinitionSet, error) {
	v, err := s.repo.Active(ctx)
	if err != nil {
		return v, fmt.Errorf("active configuration: %w", err)
	}
	return v, nil
}
func (s *Service) Versions(ctx context.Context) ([]quota.DefinitionSet, error) {
	return s.repo.ListVersions(ctx)
}
func (s *Service) SetShadow(ctx context.Context, version int64) error {
	v, err := s.repo.GetVersion(ctx, version)
	if err != nil {
		return fmt.Errorf("shadow version: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shadow = &v
	return nil
}
func (s *Service) Shadow() *quota.DefinitionSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shadow == nil {
		return nil
	}
	v := *s.shadow
	v.Definitions = append([]quota.Definition(nil), s.shadow.Definitions...)
	return &v
}

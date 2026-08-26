package infrastructure

import (
	"context"
	cfg "github.com/enterprise-labs/quota-decision-plane/internal/configuration/domain"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
	"sort"
	"sync"
)

type MemoryRepository struct {
	mu       sync.RWMutex
	drafts   map[string]cfg.Draft
	versions map[int64]quota.DefinitionSet
	active   int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{drafts: map[string]cfg.Draft{}, versions: map[int64]quota.DefinitionSet{}}
}
func (r *MemoryRepository) SaveDraft(ctx context.Context, d cfg.Draft) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d.Definitions = append([]quota.Definition(nil), d.Definitions...)
	r.drafts[d.ID] = d
	return nil
}
func (r *MemoryRepository) GetDraft(ctx context.Context, id string) (cfg.Draft, error) {
	if err := ctx.Err(); err != nil {
		return cfg.Draft{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drafts[id]
	if !ok {
		return d, cfg.ErrDraftNotFound
	}
	d.Definitions = append([]quota.Definition(nil), d.Definitions...)
	return d, nil
}
func (r *MemoryRepository) DeleteDraft(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.drafts[id]; !ok {
		return cfg.ErrDraftNotFound
	}
	delete(r.drafts, id)
	return nil
}
func (r *MemoryRepository) SaveVersion(ctx context.Context, v quota.DefinitionSet) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v.Definitions = append([]quota.Definition(nil), v.Definitions...)
	r.versions[v.Version] = v
	return nil
}
func (r *MemoryRepository) GetVersion(ctx context.Context, n int64) (quota.DefinitionSet, error) {
	if err := ctx.Err(); err != nil {
		return quota.DefinitionSet{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.versions[n]
	if !ok {
		return v, cfg.ErrVersionNotFound
	}
	v.Definitions = append([]quota.Definition(nil), v.Definitions...)
	return v, nil
}
func (r *MemoryRepository) ListVersions(ctx context.Context) ([]quota.DefinitionSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]quota.DefinitionSet, 0, len(r.versions))
	for _, v := range r.versions {
		v.Definitions = append([]quota.Definition(nil), v.Definitions...)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
func (r *MemoryRepository) SetActive(ctx context.Context, n int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.versions[n]; !ok {
		return cfg.ErrVersionNotFound
	}
	r.active = n
	return nil
}
func (r *MemoryRepository) Active(ctx context.Context) (quota.DefinitionSet, error) {
	r.mu.RLock()
	n := r.active
	r.mu.RUnlock()
	return r.GetVersion(ctx, n)
}

package application

import (
	"errors"
	"fmt"
	"strings"

	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
)

var ErrQuotaNotFound = errors.New("matching quota not found")

type Matcher struct{}

func NewMatcher() *Matcher { return &Matcher{} }

func (m *Matcher) Match(definitions []quota.Definition, request quota.DecisionRequest) (quota.Definition, error) {
	items := append([]quota.Definition(nil), definitions...)
	quota.SortDefinitions(items)
	for _, item := range items {
		if !item.Enabled || !strings.EqualFold(item.TenantID, request.TenantID) || !strings.EqualFold(item.Resource, request.Resource) {
			continue
		}
		if item.Dimensions.Matches(request.Dimensions) {
			return item, nil
		}
	}
	return quota.Definition{}, fmt.Errorf("%w for %s", ErrQuotaNotFound, request.ScopeKey())
}

func (m *Matcher) Chain(definitions []quota.Definition, leaf quota.Definition) ([]quota.Definition, error) {
	byID := make(map[string]quota.Definition, len(definitions))
	for _, d := range definitions {
		byID[d.ID] = d
	}
	seen := map[string]bool{}
	chain := []quota.Definition{leaf}
	current := leaf
	for current.ParentID != "" {
		if seen[current.ID] {
			return nil, fmt.Errorf("quota parent cycle at %s", current.ID)
		}
		seen[current.ID] = true
		parent, ok := byID[current.ParentID]
		if !ok {
			return nil, fmt.Errorf("parent quota %s not found", current.ParentID)
		}
		chain = append(chain, parent)
		current = parent
	}
	return chain, nil
}

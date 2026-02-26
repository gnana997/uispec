package catalog

import "strings"

// ComponentSearchResult holds a component match with the reason it matched.
type ComponentSearchResult struct {
	Component   *Component
	MatchReason string
}

// QueryService provides read-only query methods over a loaded catalog.
type QueryService struct {
	Catalog *Catalog
	Index   *CatalogIndex
}

// NewQueryService creates a QueryService from a validated catalog and its index.
func NewQueryService(cat *Catalog, idx *CatalogIndex) *QueryService {
	return &QueryService{Catalog: cat, Index: idx}
}

// LoadAndQuery loads a catalog from file and returns a ready-to-use QueryService.
func LoadAndQuery(path string) (*QueryService, error) {
	cat, idx, err := LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	return NewQueryService(cat, idx), nil
}

// LoadAndQueryBytes loads a catalog from raw JSON bytes and returns a ready-to-use QueryService.
func LoadAndQueryBytes(data []byte) (*QueryService, error) {
	cat, idx, err := LoadFromBytes(data)
	if err != nil {
		return nil, err
	}
	return NewQueryService(cat, idx), nil
}

// ListCategories returns all categories in the catalog.
func (q *QueryService) ListCategories() []Category {
	return q.Catalog.Categories
}

// ListComponents returns components filtered by category and/or keyword.
// Both filters are optional (pass "" to skip). When both are provided, they combine with AND logic.
// The keyword matches case-insensitively against component Name and Description.
func (q *QueryService) ListComponents(category, keyword string) []Component {
	var candidates []*Component

	if category != "" {
		candidates = q.Index.ComponentsByCategory[category]
	} else {
		candidates = make([]*Component, 0, len(q.Catalog.Components))
		for i := range q.Catalog.Components {
			candidates = append(candidates, &q.Catalog.Components[i])
		}
	}

	keyword = strings.ToLower(keyword)
	result := make([]Component, 0)

	for _, comp := range candidates {
		if keyword != "" {
			nameLower := strings.ToLower(comp.Name)
			descLower := strings.ToLower(comp.Description)
			if !strings.Contains(nameLower, keyword) && !strings.Contains(descLower, keyword) {
				continue
			}
		}
		result = append(result, *comp)
	}

	return result
}

// GetComponent looks up a component by name. It first checks top-level components,
// then falls back to sub-component names (returning the parent component).
// The bool indicates whether the component was found.
func (q *QueryService) GetComponent(name string) (*Component, bool) {
	if comp, ok := q.Index.ComponentByName[name]; ok {
		return comp, true
	}
	if parent, ok := q.Index.SubComponentByName[name]; ok {
		return parent, true
	}
	return nil, false
}

// GetComponentsByNames returns components matching the given names.
// Unknown names are silently skipped. Duplicates are removed.
func (q *QueryService) GetComponentsByNames(names []string) []*Component {
	seen := make(map[string]bool, len(names))
	result := make([]*Component, 0, len(names))

	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		if comp, ok := q.Index.ComponentByName[name]; ok {
			result = append(result, comp)
		}
	}

	return result
}

// GetTokens returns design tokens, optionally filtered by category.
// Pass "" to return all tokens.
func (q *QueryService) GetTokens(category string) []Token {
	if category == "" {
		return q.Catalog.Tokens
	}
	result := make([]Token, 0)
	for _, t := range q.Catalog.Tokens {
		if t.Category == category {
			result = append(result, t)
		}
	}
	return result
}

// GetGuidelines returns guidelines. If component is empty, returns only global guidelines.
// If component is non-empty, returns global guidelines merged with that component's guidelines.
func (q *QueryService) GetGuidelines(component string) []Guideline {
	if component == "" {
		return q.Catalog.Guidelines
	}

	comp, ok := q.Index.ComponentByName[component]
	if !ok {
		return q.Catalog.Guidelines
	}

	merged := make([]Guideline, 0, len(q.Catalog.Guidelines)+len(comp.Guidelines))
	merged = append(merged, q.Catalog.Guidelines...)
	merged = append(merged, comp.Guidelines...)
	return merged
}

// SearchComponents performs a case-insensitive search across component names,
// descriptions, prop names, and sub-component names.
// Returns matching components with the reason for the match.
func (q *QueryService) SearchComponents(query string) []ComponentSearchResult {
	query = strings.ToLower(query)
	if query == "" {
		return nil
	}

	seen := make(map[string]bool)
	var results []ComponentSearchResult

	for i := range q.Catalog.Components {
		comp := &q.Catalog.Components[i]

		if strings.Contains(strings.ToLower(comp.Name), query) {
			if !seen[comp.Name] {
				seen[comp.Name] = true
				results = append(results, ComponentSearchResult{Component: comp, MatchReason: "name"})
			}
			continue
		}

		if strings.Contains(strings.ToLower(comp.Description), query) {
			if !seen[comp.Name] {
				seen[comp.Name] = true
				results = append(results, ComponentSearchResult{Component: comp, MatchReason: "description"})
			}
			continue
		}

		for _, prop := range comp.Props {
			if strings.Contains(strings.ToLower(prop.Name), query) {
				if !seen[comp.Name] {
					seen[comp.Name] = true
					results = append(results, ComponentSearchResult{Component: comp, MatchReason: "prop:" + prop.Name})
				}
				break
			}
		}

		if seen[comp.Name] {
			continue
		}

		for _, sub := range comp.SubComponents {
			if strings.Contains(strings.ToLower(sub.Name), query) {
				if !seen[comp.Name] {
					seen[comp.Name] = true
					results = append(results, ComponentSearchResult{Component: comp, MatchReason: "sub-component:" + sub.Name})
				}
				break
			}
		}
	}

	return results
}

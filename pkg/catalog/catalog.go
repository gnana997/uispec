package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Catalog holds the full design system specification.
type Catalog struct {
	Name       string      `json:"name"`
	Version    string      `json:"version"`
	Framework  string      `json:"framework"`
	Source     string      `json:"source"`
	Components []Component `json:"components"`
	Tokens     []Token     `json:"tokens"`
	Guidelines []Guideline `json:"guidelines"`
	Categories []Category  `json:"categories"`
}

// CatalogIndex provides O(1) lookups into the catalog.
// Built during LoadFromFile after validation passes.
type CatalogIndex struct {
	// ComponentByName maps component name -> *Component.
	ComponentByName map[string]*Component

	// SubComponentByName maps sub-component name -> parent *Component.
	SubComponentByName map[string]*Component

	// SubComponentDef maps sub-component name -> *SubComponent definition.
	SubComponentDef map[string]*SubComponent

	// CategoryByName maps category name -> *Category.
	CategoryByName map[string]*Category

	// ComponentsByCategory maps category name -> []*Component.
	ComponentsByCategory map[string][]*Component
}

// validSeverities defines the allowed severity values.
var validSeverities = map[string]bool{
	"error":   true,
	"warning": true,
	"info":    true,
}

// Validate checks the catalog for internal consistency.
// Returns a slice of validation errors (empty slice if valid).
func (c *Catalog) Validate() []error {
	var errs []error

	// Required catalog-level fields.
	if c.Name == "" {
		errs = append(errs, fmt.Errorf("catalog name is required"))
	}
	if c.Version == "" {
		errs = append(errs, fmt.Errorf("catalog version is required"))
	}

	// Build sets for duplicate detection.
	componentNames := make(map[string]bool, len(c.Components))
	allSubComponentNames := make(map[string]bool)
	categoryNames := make(map[string]bool, len(c.Categories))

	// Validate categories.
	for i, cat := range c.Categories {
		if cat.Name == "" {
			errs = append(errs, fmt.Errorf("categories[%d]: name is required", i))
			continue
		}
		if categoryNames[cat.Name] {
			errs = append(errs, fmt.Errorf("categories[%d]: duplicate category name %q", i, cat.Name))
			continue
		}
		categoryNames[cat.Name] = true
	}

	// Validate components.
	for i, comp := range c.Components {
		if comp.Name == "" {
			errs = append(errs, fmt.Errorf("components[%d]: name is required", i))
			continue
		}
		if comp.ImportPath == "" {
			errs = append(errs, fmt.Errorf("component %q: import_path is required", comp.Name))
		}
		if len(comp.ImportedNames) == 0 {
			errs = append(errs, fmt.Errorf("component %q: imported_names must have at least one entry", comp.Name))
		}
		if componentNames[comp.Name] {
			errs = append(errs, fmt.Errorf("component %q: duplicate component name", comp.Name))
			continue
		}
		componentNames[comp.Name] = true

		// Category reference.
		if comp.Category != "" && !categoryNames[comp.Category] {
			errs = append(errs, fmt.Errorf("component %q: references unknown category %q", comp.Name, comp.Category))
		}

		// Validate props.
		for j, prop := range comp.Props {
			if prop.Name == "" {
				errs = append(errs, fmt.Errorf("component %q props[%d]: name is required", comp.Name, j))
			}
			if prop.Type == "" {
				errs = append(errs, fmt.Errorf("component %q props[%d]: type is required", comp.Name, j))
			}
		}

		// Validate sub-components.
		for j, sub := range comp.SubComponents {
			if sub.Name == "" {
				errs = append(errs, fmt.Errorf("component %q sub_components[%d]: name is required", comp.Name, j))
				continue
			}
			if allSubComponentNames[sub.Name] {
				errs = append(errs, fmt.Errorf("component %q: duplicate sub-component name %q", comp.Name, sub.Name))
				continue
			}
			if componentNames[sub.Name] {
				errs = append(errs, fmt.Errorf("component %q: sub-component name %q collides with a top-level component", comp.Name, sub.Name))
				continue
			}
			allSubComponentNames[sub.Name] = true

			// Validate sub-component props.
			for k, prop := range sub.Props {
				if prop.Name == "" {
					errs = append(errs, fmt.Errorf("component %q sub-component %q props[%d]: name is required", comp.Name, sub.Name, k))
				}
				if prop.Type == "" {
					errs = append(errs, fmt.Errorf("component %q sub-component %q props[%d]: type is required", comp.Name, sub.Name, k))
				}
			}
		}

		// Validate component-level guidelines.
		for j, g := range comp.Guidelines {
			if !validSeverities[g.Severity] {
				errs = append(errs, fmt.Errorf("component %q guidelines[%d]: invalid severity %q (must be error/warning/info)", comp.Name, j, g.Severity))
			}
		}
	}

	// Validate global guidelines.
	for i, g := range c.Guidelines {
		if !validSeverities[g.Severity] {
			errs = append(errs, fmt.Errorf("guidelines[%d]: invalid severity %q (must be error/warning/info)", i, g.Severity))
		}
	}

	// Cross-reference: each component listed in a category must exist.
	for _, cat := range c.Categories {
		for _, compName := range cat.Components {
			if !componentNames[compName] {
				errs = append(errs, fmt.Errorf("category %q: references non-existent component %q", cat.Name, compName))
			}
		}
	}

	return errs
}

// BuildIndex creates lookup maps for fast access.
// Should be called after Validate() passes.
func (c *Catalog) BuildIndex() *CatalogIndex {
	idx := &CatalogIndex{
		ComponentByName:      make(map[string]*Component, len(c.Components)),
		SubComponentByName:   make(map[string]*Component),
		SubComponentDef:      make(map[string]*SubComponent),
		CategoryByName:       make(map[string]*Category, len(c.Categories)),
		ComponentsByCategory: make(map[string][]*Component),
	}

	for i := range c.Categories {
		idx.CategoryByName[c.Categories[i].Name] = &c.Categories[i]
	}

	for i := range c.Components {
		comp := &c.Components[i]
		idx.ComponentByName[comp.Name] = comp
		idx.ComponentsByCategory[comp.Category] = append(idx.ComponentsByCategory[comp.Category], comp)

		for j := range comp.SubComponents {
			sub := &comp.SubComponents[j]
			idx.SubComponentByName[sub.Name] = comp
			idx.SubComponentDef[sub.Name] = sub
		}
	}

	return idx
}

// LoadFromFile loads a catalog from a JSON file, validates it, and builds the index.
func LoadFromFile(path string) (*Catalog, *CatalogIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read catalog file: %w", err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses a catalog from raw JSON bytes, validates it, and builds the index.
func LoadFromBytes(data []byte) (*Catalog, *CatalogIndex, error) {
	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, nil, fmt.Errorf("failed to parse catalog JSON: %w", err)
	}

	if errs := catalog.Validate(); len(errs) > 0 {
		return nil, nil, fmt.Errorf("catalog validation failed: %w", errors.Join(errs...))
	}

	index := catalog.BuildIndex()
	return &catalog, index, nil
}

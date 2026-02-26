package scanner

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/gnana997/uispec/pkg/catalog"
)

// BuildCatalog assembles a catalog.Catalog from scan results and extracted props.
func BuildCatalog(
	scanResult *ScanResult,
	propsMap map[string]*PropExtractionResult,
	cfg CatalogBuildConfig,
) (*catalog.Catalog, error) {
	// Build set of sub-component names.
	subComponentSet := make(map[string]bool)
	groupByParent := make(map[string]*CompoundGroup)
	for i := range scanResult.CompoundGroups {
		g := &scanResult.CompoundGroups[i]
		groupByParent[g.Parent.Name] = g
		for _, sub := range g.SubComponents {
			subComponentSet[sub.Name] = true
		}
	}

	// Build components grouped by category.
	categoryComponents := make(map[string][]string)
	var components []catalog.Component

	for _, dc := range scanResult.Components {
		// Skip sub-components — they'll be nested under their parent.
		if subComponentSet[dc.Name] {
			continue
		}

		comp := buildComponent(dc, propsMap, groupByParent[dc.Name], cfg)

		cat := computeCategory(dc.FilePath, cfg.RootDir)
		comp.Category = cat
		components = append(components, comp)

		categoryComponents[cat] = append(categoryComponents[cat], dc.Name)
	}

	// Sort components by name.
	sort.Slice(components, func(i, j int) bool {
		return components[i].Name < components[j].Name
	})

	// Build categories.
	categories := buildCategories(categoryComponents)

	// Catalog metadata.
	name := cfg.Name
	if name == "" {
		name = filepath.Base(cfg.RootDir)
	}

	cat := &catalog.Catalog{
		Name:       name,
		Version:    "1.0",
		Framework:  "react",
		Source:     "uispec scan",
		Components: components,
		Categories: categories,
	}

	// Validate.
	if errs := cat.Validate(); len(errs) > 0 {
		// Return the catalog anyway, with the first error.
		return cat, errs[0]
	}

	return cat, nil
}

// buildComponent builds a catalog.Component from a DetectedComponent.
func buildComponent(
	dc DetectedComponent,
	propsMap map[string]*PropExtractionResult,
	group *CompoundGroup,
	cfg CatalogBuildConfig,
) catalog.Component {
	comp := catalog.Component{
		Name:       dc.Name,
		ImportPath: computeImportPath(dc.FilePath, cfg),
	}

	// ImportedNames: self + sub-components.
	comp.ImportedNames = []string{dc.Name}

	// Props from extraction.
	if pr, ok := propsMap[dc.Name]; ok {
		comp.Props = convertProps(pr.Props)
	}

	// Sub-components.
	if group != nil {
		for _, sub := range group.SubComponents {
			comp.ImportedNames = append(comp.ImportedNames, sub.Name)

			subComp := catalog.SubComponent{
				Name:           sub.Name,
				AllowedParents: []string{dc.Name},
			}

			if pr, ok := propsMap[sub.Name]; ok {
				subComp.Props = convertProps(pr.Props)
			}

			comp.SubComponents = append(comp.SubComponents, subComp)
		}
	}

	return comp
}

// convertProps converts ExtractedProp slice to catalog.Prop slice.
func convertProps(extracted []ExtractedProp) []catalog.Prop {
	if len(extracted) == 0 {
		return nil
	}
	props := make([]catalog.Prop, len(extracted))
	for i, ep := range extracted {
		props[i] = catalog.Prop{
			Name:          ep.Name,
			Type:          ep.Type,
			Required:      ep.Required,
			Default:       ep.Default,
			Description:   ep.Description,
			AllowedValues: ep.AllowedValues,
			Deprecated:    ep.Deprecated,
		}
	}
	return props
}

// computeImportPath computes the import path for a component file.
func computeImportPath(filePath string, cfg CatalogBuildConfig) string {
	rootDir := cfg.RootDir
	if rootDir == "" {
		return filePath
	}

	// Make both absolute for reliable Rel computation.
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return filePath
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return filePath
	}

	rel, err := filepath.Rel(absRoot, absFile)
	if err != nil {
		return filePath
	}

	// Use forward slashes.
	rel = filepath.ToSlash(rel)

	// Strip extension.
	ext := filepath.Ext(rel)
	rel = strings.TrimSuffix(rel, ext)

	// Strip /index suffix.
	rel = strings.TrimSuffix(rel, "/index")

	// Prepend import prefix.
	if cfg.ImportPrefix != "" {
		prefix := strings.TrimSuffix(cfg.ImportPrefix, "/")
		return prefix + "/" + rel
	}

	return "./" + rel
}

// computeCategory determines a category name based on subdirectory.
func computeCategory(filePath string, rootDir string) string {
	if rootDir == "" {
		return "components"
	}

	absFile, _ := filepath.Abs(filePath)
	absRoot, _ := filepath.Abs(rootDir)
	rel, err := filepath.Rel(absRoot, absFile)
	if err != nil {
		return "components"
	}

	// Get the directory portion.
	dir := filepath.Dir(rel)
	if dir == "." || dir == "" {
		return "components"
	}

	// Use the first directory segment as the category.
	parts := strings.Split(filepath.ToSlash(dir), "/")
	return parts[0]
}

// buildCategories creates sorted catalog.Category entries from a name→components map.
func buildCategories(categoryComponents map[string][]string) []catalog.Category {
	var categories []catalog.Category
	for name, compNames := range categoryComponents {
		sort.Strings(compNames)
		categories = append(categories, catalog.Category{
			Name:       name,
			Components: compNames,
		})
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Name < categories[j].Name
	})
	return categories
}

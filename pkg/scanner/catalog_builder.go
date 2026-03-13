package scanner

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/gnana997/uispec/pkg/catalog"
)

// BuildCatalog assembles a catalog.Catalog from scan results and extracted props.
func BuildCatalog(
	scanResult *ScanResult,
	propsMap map[string]*PropExtractionResult,
	cfg CatalogBuildConfig,
	tokens []catalog.Token,
	examples map[string][]catalog.Example,
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

	// Detect duplicate component names and disambiguate them using the file stem.
	nameCount := make(map[string]int)
	for _, dc := range scanResult.Components {
		if !subComponentSet[dc.Name] {
			nameCount[dc.Name]++
		}
	}

	// Build components grouped by category.
	categoryComponents := make(map[string][]string)
	var components []catalog.Component
	usedNames := make(map[string]bool)

	for _, dc := range scanResult.Components {
		// Skip sub-components — they'll be nested under their parent.
		if subComponentSet[dc.Name] {
			continue
		}

		name := dc.Name
		if nameCount[name] > 1 {
			// Disambiguate: prefix with PascalCase file stem.
			// e.g., "Calendar" from "date-range-picker.tsx" → "DateRangePickerCalendar"
			stem := fileBaseStem(dc.FilePath)
			candidate := stem + name
			if !usedNames[candidate] {
				name = candidate
			}
			// If still collides (same file stem), keep original — validation will catch it.
		}
		usedNames[name] = true

		dcCopy := dc
		dcCopy.Name = name
		comp := buildComponent(dcCopy, propsMap, groupByParent[dc.Name], cfg, examples)

		cat := computeCategory(dc.FilePath, cfg.RootDir, cfg.CategoryRules)
		comp.Category = cat
		components = append(components, comp)

		categoryComponents[cat] = append(categoryComponents[cat], name)
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
		Tokens:     tokens,
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
	examples map[string][]catalog.Example,
) catalog.Component {
	comp := catalog.Component{
		Name:       dc.Name,
		ImportPath: computeImportPath(dc.FilePath, cfg),
	}

	// ImportedNames: self + sub-components.
	comp.ImportedNames = []string{dc.Name}

	// Props and description from extraction.
	if pr, ok := propsMap[dc.Name]; ok {
		comp.Props = convertProps(pr.Props)
		if pr.Description != "" {
			comp.Description = pr.Description
		}
	}

	// Examples from Storybook extraction.
	if exs, ok := examples[dc.Name]; ok {
		comp.Examples = exs
	}

	// Sub-components.
	if group != nil {
		seenSubs := make(map[string]bool)
		for _, sub := range group.SubComponents {
			if seenSubs[sub.Name] {
				continue
			}
			seenSubs[sub.Name] = true

			comp.ImportedNames = append(comp.ImportedNames, sub.Name)

			subComp := catalog.SubComponent{
				Name:           sub.Name,
				AllowedParents: []string{dc.Name},
			}

			if pr, ok := propsMap[sub.Name]; ok {
				subComp.Props = convertProps(pr.Props)
				if pr.Description != "" {
					subComp.Description = pr.Description
				}
			}

			comp.SubComponents = append(comp.SubComponents, subComp)
		}
	}

	// Fallback description when none was provided by enrichment or storybook.
	if comp.Description == "" {
		comp.Description = generateFallbackDescription(comp)
	}

	return comp
}

// knownRoles maps component name patterns to human-readable UI role descriptions.
var knownRoles = map[string]string{
	"button":         "A clickable button element",
	"input":          "A text input field",
	"textarea":       "A multi-line text input",
	"select":         "A selection dropdown",
	"checkbox":       "A checkbox input",
	"switch":         "A toggle switch",
	"radio":          "A radio button group",
	"slider":         "A range slider",
	"label":          "A form label",
	"form":           "A form component",
	"dialog":         "A modal dialog overlay",
	"alert":          "An alert notification",
	"toast":          "A toast notification",
	"tooltip":        "A tooltip popup",
	"popover":        "A popover overlay",
	"dropdown":       "A dropdown menu",
	"menu":           "A menu component",
	"menubar":        "A horizontal menu bar",
	"context":        "A context menu",
	"navigation":     "A navigation menu",
	"tabs":           "A tabbed interface",
	"accordion":      "An expandable accordion",
	"collapsible":    "A collapsible section",
	"card":           "A card container",
	"table":          "A data table",
	"badge":          "A badge label",
	"avatar":         "An avatar display",
	"separator":      "A visual separator",
	"skeleton":       "A loading skeleton placeholder",
	"spinner":        "A loading spinner",
	"progress":       "A progress indicator",
	"breadcrumb":     "A breadcrumb navigation trail",
	"pagination":     "A pagination control",
	"carousel":       "A content carousel",
	"calendar":       "A date calendar picker",
	"scroll":         "A scrollable area",
	"sidebar":        "A sidebar navigation panel",
	"sheet":          "A slide-out sheet panel",
	"drawer":         "A drawer panel",
	"hover":          "A hover-triggered card",
	"toggle":         "A toggle button",
	"command":        "A command palette",
	"combobox":       "A combobox with search and selection",
	"resizable":      "A resizable panel",
	"aspect":         "An aspect ratio container",
	"field":          "A form field wrapper",
	"empty":          "An empty state placeholder",
	"kbd":            "A keyboard shortcut display",
	"item":           "A list item container",
	"chart":          "A chart visualization",
	"otp":            "A one-time password input",
}

// splitCamelCase splits a PascalCase name into lowercase words.
// e.g. "AlertDialog" → ["alert", "dialog"], "HoverCard" → ["hover", "card"]
var camelRe = regexp.MustCompile(`[A-Z][a-z]*`)

func splitCamelCase(name string) []string {
	parts := camelRe.FindAllString(name, -1)
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	return parts
}

// humanizeName converts a PascalCase component name to readable form.
// e.g. "AlertDialog" → "alert dialog", "HoverCard" → "hover card"
func humanizeName(name string) string {
	return strings.Join(splitCamelCase(name), " ")
}

// detectRole finds a known UI role from a component name.
// It tries the full lowercase name first, then progressively shorter prefixes
// of the camelCase words. Returns the role description and true if found.
func detectRole(name string) (string, bool) {
	lower := strings.ToLower(name)
	if role, ok := knownRoles[lower]; ok {
		return role, true
	}

	words := splitCamelCase(name)
	// Try progressively shorter prefixes: ["alert", "dialog"] → "alertdialog", then "alert"
	for n := len(words); n > 0; n-- {
		key := strings.Join(words[:n], "")
		if role, ok := knownRoles[key]; ok {
			return role, true
		}
	}
	// Try single words (for compound names like ContextMenu → "context" or "menu")
	for _, w := range words {
		if role, ok := knownRoles[w]; ok {
			return role, true
		}
	}
	return "", false
}

// generateFallbackDescription builds a short description from the component's
// name, props, and sub-components when no docstring or storybook description exists.
func generateFallbackDescription(comp catalog.Component) string {
	var desc string

	// Start with role-based or generic opener.
	if role, ok := detectRole(comp.Name); ok {
		desc = role
	} else {
		desc = "A " + humanizeName(comp.Name) + " component"
	}

	// Collect trait phrases from props.
	var traits []string

	propSet := make(map[string]bool, len(comp.Props))
	var variantValues []string
	for _, p := range comp.Props {
		propSet[p.Name] = true
		if p.Name == "variant" && len(p.AllowedValues) > 0 {
			variantValues = p.AllowedValues
		}
	}

	// Variant/size support.
	if len(variantValues) > 0 {
		if len(variantValues) <= 5 {
			traits = append(traits, "available in "+joinWords(variantValues)+" variants")
		} else {
			traits = append(traits, fmt.Sprintf("available in %d variants", len(variantValues)))
		}
	} else if propSet["variant"] {
		traits = append(traits, "supports multiple variants")
	}
	if propSet["size"] {
		traits = append(traits, "with configurable sizing")
	}

	// Controlled state.
	if propSet["open"] && propSet["onOpenChange"] {
		traits = append(traits, "with controlled open/close state")
	} else if propSet["value"] && propSet["onValueChange"] {
		traits = append(traits, "with controlled value state")
	} else if propSet["checked"] && propSet["onCheckedChange"] {
		traits = append(traits, "with controlled checked state")
	} else if propSet["pressed"] && propSet["onPressedChange"] {
		traits = append(traits, "with controlled pressed state")
	}

	// Form-related.
	if propSet["disabled"] && !propSet["open"] {
		// Only mention disabled for form-like components, not overlays.
		traits = append(traits, "supports disabled state")
	}

	// Compound composition.
	if len(comp.SubComponents) > 0 {
		subNames := make([]string, len(comp.SubComponents))
		for i, sc := range comp.SubComponents {
			subNames[i] = sc.Name
		}
		if len(subNames) <= 4 {
			traits = append(traits, "composed of "+joinWords(subNames))
		} else {
			traits = append(traits, fmt.Sprintf("composed of %d sub-components including %s",
				len(subNames), joinWords(subNames[:3])))
		}
	}

	// Assemble: "A modal dialog overlay, with controlled open/close state, composed of ..."
	if len(traits) > 0 {
		// Capitalize the first trait connector.
		desc += ", " + strings.Join(traits, ", ")
	}
	desc += "."

	// Ensure first letter is uppercase.
	if len(desc) > 0 {
		runes := []rune(desc)
		runes[0] = unicode.ToUpper(runes[0])
		desc = string(runes)
	}

	return desc
}

// joinWords joins strings with commas and "and" before the last item.
// e.g. ["a", "b", "c"] → "a, b, and c"
func joinWords(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
	}
}

// fileBaseStem extracts the file name without extension and converts it to PascalCase.
// e.g., "date-range-picker.tsx" → "DateRangePicker", "calendar.tsx" → "Calendar"
func fileBaseStem(filePath string) string {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	// Strip /index — not useful for disambiguation.
	if stem == "index" {
		dir := filepath.Dir(filePath)
		stem = filepath.Base(dir)
	}
	// Convert kebab-case/snake_case to PascalCase.
	var result strings.Builder
	capitalize := true
	for _, r := range stem {
		if r == '-' || r == '_' || r == '.' {
			capitalize = true
			continue
		}
		if capitalize {
			result.WriteRune(unicode.ToUpper(r))
			capitalize = false
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// universalProps are present on virtually every React component.
// They add noise to the catalog without useful information.
var universalProps = map[string]bool{
	"className": true,
	"style":     true,
	"ref":       true,
	"key":       true,
}

// convertProps converts ExtractedProp slice to catalog.Prop slice,
// filtering out universal props that are noise in a catalog.
func convertProps(extracted []ExtractedProp) []catalog.Prop {
	if len(extracted) == 0 {
		return nil
	}
	var props []catalog.Prop
	for _, ep := range extracted {
		if universalProps[ep.Name] {
			continue
		}
		props = append(props, catalog.Prop{
			Name:          ep.Name,
			Type:          ep.Type,
			Required:      ep.Required,
			Default:       ep.Default,
			Description:   ep.Description,
			AllowedValues: ep.AllowedValues,
			Deprecated:    ep.Deprecated,
		})
	}
	if len(props) == 0 {
		return nil
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

// computeCategory determines a category name. If rules are provided, tries
// glob matching first (first match wins), then falls back to subdirectory heuristic.
func computeCategory(filePath string, rootDir string, rules []CategoryRule) string {
	if rootDir == "" {
		return "components"
	}

	absFile, _ := filepath.Abs(filePath)
	absRoot, _ := filepath.Abs(rootDir)
	rel, err := filepath.Rel(absRoot, absFile)
	if err != nil {
		return "components"
	}
	rel = filepath.ToSlash(rel)

	// Try manual category rules first.
	for _, rule := range rules {
		if matched, _ := doublestar.PathMatch(rule.Pattern, rel); matched {
			return rule.Name
		}
	}

	// Fall back to subdirectory heuristic.
	dir := filepath.Dir(rel)
	if dir == "." || dir == "" {
		return "components"
	}

	// Use the first directory segment as the category.
	parts := strings.Split(dir, "/")
	return parts[0]
}

// ParseCategoryRules parses a comma-separated "name=glob,name2=glob2" string.
func ParseCategoryRules(s string) ([]CategoryRule, error) {
	if s == "" {
		return nil, nil
	}
	var rules []CategoryRule
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq < 1 || eq == len(part)-1 {
			return nil, fmt.Errorf("invalid category rule %q: expected name=glob", part)
		}
		rules = append(rules, CategoryRule{
			Name:    strings.TrimSpace(part[:eq]),
			Pattern: strings.TrimSpace(part[eq+1:]),
		})
	}
	return rules, nil
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

package scanner

import "strings"

// MergeEnrichedProps merges Node.js enrichment data into tree-sitter-extracted props.
//
// Merge strategy:
//   - Tree-sitter props are the base (they have CVA variants and destructuring defaults).
//   - Node enrichment fills empty fields (description, required, deprecated).
//   - Props only found by Node (inherited from extended interfaces) are added.
//   - Component-level description is set from Node if tree-sitter didn't find one.
func MergeEnrichedProps(
	propsMap map[string]*PropExtractionResult,
	enriched *EnrichResult,
	components []DetectedComponent,
) {
	if enriched == nil || len(enriched.Components) == 0 {
		return
	}

	for _, comp := range components {
		docgen, ok := enriched.Components[comp.Name]
		if !ok {
			continue
		}

		pr, ok := propsMap[comp.Name]
		if !ok {
			// Node found a component that tree-sitter didn't extract props for.
			// Create a new result with Node's props.
			pr = &PropExtractionResult{
				ComponentName: comp.Name,
				FilePath:      comp.FilePath,
			}
			propsMap[comp.Name] = pr
		}

		pr.Props = mergeProps(pr.Props, docgen.Props)

		// Store component description for later use by catalog builder.
		if docgen.Description != "" {
			pr.Description = docgen.Description
		}
	}
}

// mergeProps merges Node-extracted props into tree-sitter-extracted props.
//
// Rules:
//   - For each tree-sitter prop, fill empty fields from matching Node prop.
//   - Node wins: Description, Required (unless tree-sitter already set it).
//   - Tree-sitter wins: AllowedValues (CVA), Default (destructuring).
//   - Add props only found by Node (inherited from extended types).
func mergeProps(base []ExtractedProp, nodeProps []DocgenProp) []ExtractedProp {
	if len(nodeProps) == 0 {
		return base
	}

	// Index base props by name.
	byName := make(map[string]int, len(base))
	for i, p := range base {
		byName[p.Name] = i
	}

	for _, np := range nodeProps {
		idx, exists := byName[np.Name]

		if exists {
			// Merge into existing prop.
			p := &base[idx]

			// Description: Node wins if tree-sitter is empty.
			if p.Description == "" && np.Description != "" {
				p.Description = np.Description
			}

			// Required: Node has better accuracy (accounts for utility types).
			// Only override if tree-sitter didn't mark it required (conservative).
			if np.Required && !p.Required {
				p.Required = np.Required
			}

			// Type: Fill if tree-sitter has no real type info, or if
			// enrichment has a more specific type than tree-sitter's generic "string"/"any",
			// or if tree-sitter has an unresolved type (PascalCase alias like "Orientation")
			// or an indexed access type (like RovingFocusGroupProps['dir']).
			enrichedType := simplifyDocgenType(np.Type)
			if np.Type != "" && (p.Type == "" || p.Type == "unknown" || p.Type == "any" ||
				(p.Type == "string" && isMoreSpecificType(enrichedType)) ||
				isUnresolvedType(p.Type) || isIndexedAccessType(p.Type)) {
				p.Type = enrichedType
			}

			// Default: Tree-sitter wins (destructuring defaults are exact).
			// Fall back to Node if tree-sitter has nothing.
			if p.Default == "" && np.DefaultValue != "" {
				p.Default = np.DefaultValue
			}

			// AllowedValues: Tree-sitter wins (CVA extraction is precise).
			// Fall back to Node if tree-sitter has nothing.
			if len(p.AllowedValues) == 0 && len(np.AllowedValues) > 0 {
				p.AllowedValues = np.AllowedValues
			}

			// Deprecated: Either source can detect it.
			if np.Deprecated {
				p.Deprecated = true
			}
		} else {
			// New prop only from Node (e.g., inherited from HTML element types).
			newProp := ExtractedProp{
				Name:          np.Name,
				Type:          simplifyDocgenType(np.Type),
				Required:      np.Required,
				Default:       np.DefaultValue,
				Description:   np.Description,
				AllowedValues: np.AllowedValues,
				Deprecated:    np.Deprecated,
			}
			base = append(base, newProp)
			byName[np.Name] = len(base) - 1
		}
	}

	return base
}

// isMoreSpecificType reports whether t is more specific than "string".
// This allows enrichment to upgrade tree-sitter's generic "string" to a real type.
func isMoreSpecificType(t string) bool {
	switch t {
	case "boolean", "number", "function", "ReactNode", "ReactElement",
		"CSSProperties", "Ref", "any":
		return true
	}
	return false
}

// isUnresolvedType reports whether a type from tree-sitter looks like an
// unresolved TypeScript type alias (e.g. "Orientation", "CheckedState",
// "Direction") that enrichment should be allowed to override.
// Matches single PascalCase identifiers that aren't known resolved types.
func isUnresolvedType(t string) bool {
	if len(t) == 0 {
		return false
	}
	// Must start with uppercase.
	if t[0] < 'A' || t[0] > 'Z' {
		return false
	}
	// Must be a single identifier (no spaces, pipes, brackets, arrows).
	for _, c := range t {
		if c == ' ' || c == '|' || c == '[' || c == '<' || c == '(' || c == '=' {
			return false
		}
	}
	// Not a known resolved type.
	switch t {
	case "ReactNode", "ReactElement", "CSSProperties", "Ref", "Element", "HTMLElement":
		return false
	}
	return true
}

// isIndexedAccessType reports whether a type from tree-sitter is an indexed
// access type like RovingFocusGroupProps['dir'] that enrichment should override.
func isIndexedAccessType(t string) bool {
	return strings.Contains(t, "['")
}

// simplifyDocgenType converts react-docgen-typescript type names to our simplified format.
func simplifyDocgenType(t string) string {
	switch t {
	case "string", "number", "boolean", "any", "void", "never", "undefined", "null",
		"object", "function", "array", "ReactNode", "ReactElement":
		return t
	case "enum":
		return "string" // enums with values become string + allowedValues
	case "() => void", "(...args: any[]) => any":
		return "function"
	case "CSSProperties", "React.CSSProperties":
		return "CSSProperties"
	case "Ref", "React.Ref":
		return "Ref"
	}

	// ReactNode, ReactElement patterns.
	if t == "React.ReactNode" || t == "ReactNode" {
		return "ReactNode"
	}
	if t == "React.ReactElement" || t == "ReactElement" {
		return "ReactElement"
	}

	// Any arrow function signature → "function".
	// Catches patterns like "(open: boolean) => void", "(value: string) => void".
	if strings.Contains(t, "=>") {
		return "function"
	}

	// Ref types: React.RefObject<T>, React.MutableRefObject<T>, etc.
	if strings.Contains(t, "Ref<") || strings.Contains(t, "RefObject") {
		return "Ref"
	}

	return t
}

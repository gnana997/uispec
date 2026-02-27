package scanner

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

			// Type: Fill if tree-sitter is empty.
			if p.Type == "" && np.Type != "" {
				p.Type = simplifyDocgenType(np.Type)
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
	}

	// ReactNode, ReactElement patterns.
	if t == "React.ReactNode" || t == "ReactNode" {
		return "ReactNode"
	}
	if t == "React.ReactElement" || t == "ReactElement" {
		return "ReactElement"
	}

	return t
}

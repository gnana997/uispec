package validator

import (
	"fmt"
	"strings"

	"github.com/gnana997/uispec/pkg/catalog"
	"github.com/gnana997/uispec/pkg/parser"
)

// Validator checks source code against the design system catalog.
type Validator struct {
	catalog *catalog.Catalog
	index   *catalog.CatalogIndex
	parser  *parser.ParserManager
}

// ValidationResult represents the result of validating a page of code.
type ValidationResult struct {
	Valid      bool        `json:"valid"`
	Violations []Violation `json:"violations"`
	Fixes      []AutoFix   `json:"fixes,omitempty"`
	FixedCode  string      `json:"fixed_code,omitempty"`
	Summary    string      `json:"summary"`
}

// Violation represents a single validation rule violation.
type Violation struct {
	Rule       string `json:"rule"`
	Message    string `json:"message"`
	Severity   string `json:"severity"` // "error", "warning", "info"
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Component  string `json:"component,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// NewValidator creates a validator backed by the given catalog and parser.
func NewValidator(cat *catalog.Catalog, idx *catalog.CatalogIndex, pm *parser.ParserManager) *Validator {
	return &Validator{
		catalog: cat,
		index:   idx,
		parser:  pm,
	}
}

// ValidatePage parses TSX code and validates component usages against the catalog.
// If autoFix is true, deterministic fixes are generated and applied.
func (v *Validator) ValidatePage(code string, autoFix bool) *ValidationResult {
	source := []byte(code)

	// Parse as TSX.
	tree, err := v.parser.Parse(source, parser.LanguageTypeScript, true)
	if err != nil {
		return &ValidationResult{
			Valid:   false,
			Summary: fmt.Sprintf("parse error: %v", err),
			Violations: []Violation{{
				Rule:     "parse-error",
				Message:  fmt.Sprintf("Failed to parse TSX: %v", err),
				Severity: "error",
				Line:     1,
				Column:   1,
			}},
		}
	}
	defer tree.Close()

	// Extract JSX usages and imports.
	extraction := ExtractJSX(tree, source)

	// Build import lookup: component name → source path.
	importedNames := make(map[string]string)  // name → source
	importSources := make(map[string][]string) // source → names
	for _, imp := range extraction.Imports {
		for _, name := range imp.Names {
			importedNames[name] = imp.Source
		}
		if imp.DefaultName != "" {
			importedNames[imp.DefaultName] = imp.Source
		}
		importSources[imp.Source] = append(importSources[imp.Source], imp.Names...)
	}

	// Track which components appear as children of which parents.
	childrenOf := make(map[string]map[string]bool) // parent → set of child component names
	for _, usage := range extraction.Usages {
		if usage.ParentComponent != "" {
			if childrenOf[usage.ParentComponent] == nil {
				childrenOf[usage.ParentComponent] = make(map[string]bool)
			}
			childrenOf[usage.ParentComponent][usage.ComponentName] = true
		}
	}

	var violations []Violation

	for _, usage := range extraction.Usages {
		comp, isTopLevel := v.index.ComponentByName[usage.ComponentName]
		_, isSubComponent := v.index.SubComponentByName[usage.ComponentName]

		if !isTopLevel && !isSubComponent {
			// Unknown component — only warn, could be a custom component.
			violations = append(violations, Violation{
				Rule:      "unknown-component",
				Message:   fmt.Sprintf("Component %q is not in the catalog", usage.ComponentName),
				Severity:  "warning",
				Line:      usage.Line,
				Column:    usage.Column,
				Component: usage.ComponentName,
			})
			continue
		}

		// Determine the catalog component for this usage.
		var catalogComp *catalog.Component
		if isTopLevel {
			catalogComp = comp
		} else {
			catalogComp = v.index.SubComponentByName[usage.ComponentName]
		}

		// Check deprecated component.
		if isTopLevel && catalogComp.Deprecated {
			msg := fmt.Sprintf("Component %q is deprecated", usage.ComponentName)
			if catalogComp.DeprecatedMsg != "" {
				msg += ": " + catalogComp.DeprecatedMsg
			}
			violations = append(violations, Violation{
				Rule:      "deprecated-component",
				Message:   msg,
				Severity:  "warning",
				Line:      usage.Line,
				Column:    usage.Column,
				Component: usage.ComponentName,
			})
		}

		// Check import (only for top-level components).
		if isTopLevel {
			violations = append(violations, v.checkImport(usage, catalogComp, importedNames)...)
		}

		// Check props (only for top-level components with defined props).
		if isTopLevel {
			violations = append(violations, v.checkProps(usage, catalogComp)...)
		}

		// Check composition rules (for sub-components).
		if isSubComponent {
			violations = append(violations, v.checkComposition(usage)...)
		}
	}

	// Check must_contain rules.
	violations = append(violations, v.checkMustContain(extraction.Usages, childrenOf)...)

	result := &ValidationResult{
		Valid:      len(filterBySeverity(violations, "error")) == 0,
		Violations: violations,
		Summary:    buildSummary(violations),
	}

	if autoFix && len(violations) > 0 {
		fixes, fixedCode := GenerateFixes(code, violations, v.index)
		if len(fixes) > 0 {
			result.Fixes = fixes
			result.FixedCode = fixedCode
		}
	}

	return result
}

// checkImport validates that the component is properly imported.
func (v *Validator) checkImport(usage JSXUsage, comp *catalog.Component, importedNames map[string]string) []Violation {
	var violations []Violation

	source, imported := importedNames[usage.ComponentName]
	if !imported {
		violations = append(violations, Violation{
			Rule:       "missing-import",
			Message:    fmt.Sprintf("Component %q is used but not imported", usage.ComponentName),
			Severity:   "error",
			Line:       usage.Line,
			Column:     usage.Column,
			Component:  usage.ComponentName,
			Suggestion: fmt.Sprintf("Add: import { %s } from %q", usage.ComponentName, comp.ImportPath),
		})
	} else if source != comp.ImportPath {
		violations = append(violations, Violation{
			Rule:       "wrong-import-path",
			Message:    fmt.Sprintf("Component %q is imported from %q but should be from %q", usage.ComponentName, source, comp.ImportPath),
			Severity:   "error",
			Line:       usage.Line,
			Column:     usage.Column,
			Component:  usage.ComponentName,
			Suggestion: fmt.Sprintf("Change import path to %q", comp.ImportPath),
		})
	}

	return violations
}

// checkProps validates component props against the catalog.
func (v *Validator) checkProps(usage JSXUsage, comp *catalog.Component) []Violation {
	var violations []Violation

	// Build prop lookup.
	propDefs := make(map[string]*catalog.Prop, len(comp.Props))
	for i := range comp.Props {
		propDefs[comp.Props[i].Name] = &comp.Props[i]
	}

	// Check for missing required props.
	for _, prop := range comp.Props {
		if prop.Required {
			if _, provided := usage.Props[prop.Name]; !provided {
				suggestion := ""
				if prop.Default != "" {
					suggestion = fmt.Sprintf("Add %s=%q", prop.Name, prop.Default)
				}
				violations = append(violations, Violation{
					Rule:       "missing-required-prop",
					Message:    fmt.Sprintf("Component %q is missing required prop %q", usage.ComponentName, prop.Name),
					Severity:   "error",
					Line:       usage.Line,
					Column:     usage.Column,
					Component:  usage.ComponentName,
					Suggestion: suggestion,
				})
			}
		}
	}

	// Check prop values and unknown props.
	for propName, propValue := range usage.Props {
		if propName == "...spread" || propName == "key" || propName == "ref" || propName == "className" || propName == "children" {
			continue // Skip React internals and spread.
		}

		def, known := propDefs[propName]
		if !known {
			violations = append(violations, Violation{
				Rule:      "unknown-prop",
				Message:   fmt.Sprintf("Prop %q is not defined for component %q", propName, usage.ComponentName),
				Severity:  "info",
				Line:      usage.Line,
				Column:    usage.Column,
				Component: usage.ComponentName,
			})
			continue
		}

		// Check deprecated prop.
		if def.Deprecated {
			violations = append(violations, Violation{
				Rule:      "deprecated-prop",
				Message:   fmt.Sprintf("Prop %q on %q is deprecated", propName, usage.ComponentName),
				Severity:  "warning",
				Line:      usage.Line,
				Column:    usage.Column,
				Component: usage.ComponentName,
			})
		}

		// Check allowed values (only for literal string values).
		if propValue != "" && propValue != "true" && len(def.AllowedValues) > 0 {
			found := false
			for _, allowed := range def.AllowedValues {
				if propValue == allowed {
					found = true
					break
				}
			}
			if !found {
				suggestion := ""
				if def.Default != "" {
					suggestion = fmt.Sprintf("Use %q instead", def.Default)
				}
				violations = append(violations, Violation{
					Rule:       "invalid-prop-value",
					Message:    fmt.Sprintf("Prop %q on %q has invalid value %q (allowed: %s)", propName, usage.ComponentName, propValue, strings.Join(def.AllowedValues, ", ")),
					Severity:   "warning",
					Line:       usage.Line,
					Column:     usage.Column,
					Component:  usage.ComponentName,
					Suggestion: suggestion,
				})
			}
		}
	}

	return violations
}

// checkComposition validates sub-component placement against allowed_parents.
func (v *Validator) checkComposition(usage JSXUsage) []Violation {
	var violations []Violation

	subDef, ok := v.index.SubComponentDef[usage.ComponentName]
	if !ok || len(subDef.AllowedParents) == 0 {
		return nil
	}

	if usage.ParentComponent == "" {
		violations = append(violations, Violation{
			Rule:       "composition-violation",
			Message:    fmt.Sprintf("%q must be a child of %s", usage.ComponentName, strings.Join(subDef.AllowedParents, " or ")),
			Severity:   "error",
			Line:       usage.Line,
			Column:     usage.Column,
			Component:  usage.ComponentName,
			Suggestion: fmt.Sprintf("Wrap in <%s>", subDef.AllowedParents[0]),
		})
		return violations
	}

	allowed := false
	for _, parent := range subDef.AllowedParents {
		if usage.ParentComponent == parent {
			allowed = true
			break
		}
	}

	if !allowed {
		violations = append(violations, Violation{
			Rule:       "composition-violation",
			Message:    fmt.Sprintf("%q is inside %q but must be a child of %s", usage.ComponentName, usage.ParentComponent, strings.Join(subDef.AllowedParents, " or ")),
			Severity:   "error",
			Line:       usage.Line,
			Column:     usage.Column,
			Component:  usage.ComponentName,
			Suggestion: fmt.Sprintf("Move inside <%s>", subDef.AllowedParents[0]),
		})
	}

	return violations
}

// checkMustContain validates that parent components contain required children.
func (v *Validator) checkMustContain(usages []JSXUsage, childrenOf map[string]map[string]bool) []Violation {
	var violations []Violation

	// For each usage that has must_contain sub-components, check children.
	for _, usage := range usages {
		subDef, ok := v.index.SubComponentDef[usage.ComponentName]
		if !ok || len(subDef.MustContain) == 0 {
			continue
		}

		children := childrenOf[usage.ComponentName]
		for _, required := range subDef.MustContain {
			if children == nil || !children[required] {
				violations = append(violations, Violation{
					Rule:       "missing-child",
					Message:    fmt.Sprintf("%q must contain a <%s> child", usage.ComponentName, required),
					Severity:   "error",
					Line:       usage.Line,
					Column:     usage.Column,
					Component:  usage.ComponentName,
					Suggestion: fmt.Sprintf("Add <%s> inside <%s>", required, usage.ComponentName),
				})
			}
		}
	}

	return violations
}

// filterBySeverity returns violations matching the given severity.
func filterBySeverity(violations []Violation, severity string) []Violation {
	var result []Violation
	for _, v := range violations {
		if v.Severity == severity {
			result = append(result, v)
		}
	}
	return result
}

// buildSummary creates a one-line summary of violations.
func buildSummary(violations []Violation) string {
	if len(violations) == 0 {
		return "no issues found"
	}

	errors := len(filterBySeverity(violations, "error"))
	warnings := len(filterBySeverity(violations, "warning"))
	infos := len(filterBySeverity(violations, "info"))

	parts := make([]string, 0, 3)
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", errors))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warnings))
	}
	if infos > 0 {
		parts = append(parts, fmt.Sprintf("%d info(s)", infos))
	}
	return strings.Join(parts, ", ")
}

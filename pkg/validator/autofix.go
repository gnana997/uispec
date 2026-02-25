package validator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gnana997/uispec/pkg/catalog"
)

// AutoFix represents a deterministic code fix that can be applied without LLM involvement.
type AutoFix struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
	Rule    string `json:"rule"`
	Reason  string `json:"reason"`
}

// GenerateFixes creates deterministic fixes for violations and returns the fixed code.
// Only violations with clear, unambiguous fixes are addressed.
func GenerateFixes(code string, violations []Violation, index *catalog.CatalogIndex) ([]AutoFix, string) {
	var fixes []AutoFix
	lines := strings.Split(code, "\n")

	// Track import insertions separately (they go at the top).
	var missingImports []string
	lastImportLine := findLastImportLine(lines)

	for _, v := range violations {
		switch v.Rule {
		case "wrong-import-path":
			fix := fixWrongImportPath(v, lines, index)
			if fix != nil {
				fixes = append(fixes, *fix)
			}

		case "missing-import":
			comp, ok := index.ComponentByName[v.Component]
			if ok {
				importLine := fmt.Sprintf("import { %s } from %q", v.Component, comp.ImportPath)
				missingImports = append(missingImports, importLine)
			}

		case "invalid-prop-value":
			fix := fixInvalidPropValue(v, lines, index)
			if fix != nil {
				fixes = append(fixes, *fix)
			}
		}
	}

	// Deduplicate missing imports by import path.
	if len(missingImports) > 0 {
		deduped := deduplicateImports(missingImports)
		insertLine := lastImportLine + 1
		if insertLine < 1 {
			insertLine = 1
		}

		for _, imp := range deduped {
			fixes = append(fixes, AutoFix{
				Line:    insertLine,
				Column:  1,
				OldText: "",
				NewText: imp,
				Rule:    "missing-import",
				Reason:  "Add missing import",
			})
		}
	}

	if len(fixes) == 0 {
		return nil, ""
	}

	fixedCode := applyFixes(code, fixes)
	return fixes, fixedCode
}

// fixWrongImportPath generates a fix for a wrong import path.
func fixWrongImportPath(v Violation, lines []string, index *catalog.CatalogIndex) *AutoFix {
	comp, ok := index.ComponentByName[v.Component]
	if !ok {
		return nil
	}

	// Find the import line containing this component.
	for i, line := range lines {
		if strings.Contains(line, v.Component) && strings.Contains(line, "import") {
			// Find the old import path in the line.
			// Look for string content between quotes.
			for _, quote := range []byte{'"', '\''} {
				start := strings.IndexByte(line, quote)
				if start < 0 {
					continue
				}
				end := strings.IndexByte(line[start+1:], quote)
				if end < 0 {
					continue
				}
				oldPath := line[start+1 : start+1+end]
				if oldPath != comp.ImportPath {
					return &AutoFix{
						Line:    i + 1,
						Column:  start + 2,
						OldText: oldPath,
						NewText: comp.ImportPath,
						Rule:    "wrong-import-path",
						Reason:  fmt.Sprintf("Fix import path for %s", v.Component),
					}
				}
			}
		}
	}
	return nil
}

// fixInvalidPropValue generates a fix for an invalid prop value.
func fixInvalidPropValue(v Violation, lines []string, index *catalog.CatalogIndex) *AutoFix {
	comp, ok := index.ComponentByName[v.Component]
	if !ok {
		return nil
	}

	// Find the prop with a default value.
	var propDef *catalog.Prop
	for i := range comp.Props {
		// Extract prop name from the message.
		if strings.Contains(v.Message, fmt.Sprintf("Prop %q", comp.Props[i].Name)) {
			propDef = &comp.Props[i]
			break
		}
	}

	if propDef == nil || propDef.Default == "" {
		return nil
	}

	// Find the invalid value in the violation message.
	// Message format: Prop "X" on "Y" has invalid value "Z" (allowed: ...)
	invalidValue := extractQuotedValue(v.Message, "invalid value")
	if invalidValue == "" {
		return nil
	}

	return &AutoFix{
		Line:    v.Line,
		Column:  v.Column,
		OldText: fmt.Sprintf(`"%s"`, invalidValue),
		NewText: fmt.Sprintf(`"%s"`, propDef.Default),
		Rule:    "invalid-prop-value",
		Reason:  fmt.Sprintf("Fix %s value from %q to %q", propDef.Name, invalidValue, propDef.Default),
	}
}

// extractQuotedValue finds a quoted value after a keyword in a message string.
func extractQuotedValue(message, keyword string) string {
	idx := strings.Index(message, keyword)
	if idx < 0 {
		return ""
	}
	rest := message[idx+len(keyword):]
	start := strings.IndexByte(rest, '"')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(rest[start+1:], '"')
	if end < 0 {
		return ""
	}
	return rest[start+1 : start+1+end]
}

// findLastImportLine returns the 1-based line number of the last import statement, or 0 if none.
func findLastImportLine(lines []string) int {
	last := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "import{") {
			last = i + 1
		}
	}
	return last
}

// deduplicateImports removes duplicate import lines.
func deduplicateImports(imports []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, imp := range imports {
		if !seen[imp] {
			seen[imp] = true
			result = append(result, imp)
		}
	}
	return result
}

// applyFixes applies all fixes to the code and returns the fixed code.
func applyFixes(code string, fixes []AutoFix) string {
	lines := strings.Split(code, "\n")

	// Separate insertions from replacements.
	var insertions []AutoFix
	var replacements []AutoFix
	for _, fix := range fixes {
		if fix.OldText == "" {
			insertions = append(insertions, fix)
		} else {
			replacements = append(replacements, fix)
		}
	}

	// Apply replacements first (in reverse line order to preserve line numbers).
	sort.Slice(replacements, func(i, j int) bool {
		return replacements[i].Line > replacements[j].Line
	})
	for _, fix := range replacements {
		lineIdx := fix.Line - 1
		if lineIdx >= 0 && lineIdx < len(lines) {
			lines[lineIdx] = strings.Replace(lines[lineIdx], fix.OldText, fix.NewText, 1)
		}
	}

	// Apply insertions (in reverse line order).
	sort.Slice(insertions, func(i, j int) bool {
		return insertions[i].Line > insertions[j].Line
	})
	for _, fix := range insertions {
		lineIdx := fix.Line - 1
		if lineIdx < 0 {
			lineIdx = 0
		}
		if lineIdx > len(lines) {
			lineIdx = len(lines)
		}
		// Insert a new line at this position.
		newLines := make([]string, len(lines)+1)
		copy(newLines, lines[:lineIdx])
		newLines[lineIdx] = fix.NewText
		copy(newLines[lineIdx+1:], lines[lineIdx:])
		lines = newLines
	}

	return strings.Join(lines, "\n")
}

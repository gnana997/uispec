package validator

import (
	"fmt"
	"strings"

	"github.com/gnana997/uispec/pkg/parser"
)

// PageAnalysis is a compact structural summary of a page's component usage.
type PageAnalysis struct {
	Components []ComponentSummary `json:"components"`
	Imports    []string           `json:"imports"`
	LineCount  int                `json:"line_count"`
}

// ComponentSummary describes one component usage in the page.
type ComponentSummary struct {
	Name     string   `json:"name"`
	Line     int      `json:"line"`
	Props    []string `json:"props"`
	Children int      `json:"children_count"`
}

// AnalyzePage parses TSX code and returns a compact structural summary.
// This is designed for the analyze_page MCP tool — gives the agent enough
// information for surgical modifications without reading the full code.
func (v *Validator) AnalyzePage(code string) *PageAnalysis {
	source := []byte(code)

	tree, err := v.parser.Parse(source, parser.LanguageTypeScript, true)
	if err != nil {
		return &PageAnalysis{
			LineCount: strings.Count(code, "\n") + 1,
		}
	}
	defer tree.Close()

	extraction := ExtractJSX(tree, source)

	// Build component summaries.
	components := make([]ComponentSummary, 0, len(extraction.Usages))
	// Count children per component (by line, to handle duplicates).
	childCount := make(map[string]int) // "Name:Line" → count of direct component children
	for _, usage := range extraction.Usages {
		if usage.ParentComponent != "" {
			// Find the parent usage to count children.
			for _, parent := range extraction.Usages {
				if parent.ComponentName == usage.ParentComponent && parent.Line <= usage.Line {
					key := fmt.Sprintf("%s:%d", parent.ComponentName, parent.Line)
					childCount[key]++
					break
				}
			}
		}
	}

	for _, usage := range extraction.Usages {
		props := make([]string, 0, len(usage.Props))
		for name := range usage.Props {
			props = append(props, name)
		}

		key := fmt.Sprintf("%s:%d", usage.ComponentName, usage.Line)
		components = append(components, ComponentSummary{
			Name:     usage.ComponentName,
			Line:     usage.Line,
			Props:    props,
			Children: childCount[key],
		})
	}

	// Build import summary (just the source paths).
	imports := make([]string, 0, len(extraction.Imports))
	for _, imp := range extraction.Imports {
		imports = append(imports, imp.Source)
	}

	return &PageAnalysis{
		Components: components,
		Imports:    imports,
		LineCount:  strings.Count(code, "\n") + 1,
	}
}

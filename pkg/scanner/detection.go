package scanner

import (
	"sort"
	"strings"
	"unicode"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/parser"
)

// DetectComponents analyzes extraction results to identify React components.
// Returns detected components and compound component groupings.
func DetectComponents(
	results []FileExtractionResult,
	pm *parser.ParserManager,
) ([]DetectedComponent, []CompoundGroup) {
	var allComponents []DetectedComponent

	for _, fer := range results {
		components := detectInFile(fer, pm)
		allComponents = append(allComponents, components...)
	}

	groups := groupCompoundComponents(allComponents)
	return allComponents, groups
}

// detectInFile analyzes a single file's extraction result for components.
func detectInFile(fer FileExtractionResult, pm *parser.ParserManager) []DetectedComponent {
	if fer.Result == nil {
		return nil
	}

	// Step A: Build candidate list — exported, uppercase, function/variable/class.
	type candidate struct {
		symbol *extractor.Symbol
		export *extractor.ExportInfo
	}
	var candidates []candidate

	// Build export lookup.
	exportMap := make(map[string]*extractor.ExportInfo)
	for i := range fer.Result.Exports {
		e := &fer.Result.Exports[i]
		exportMap[e.Name] = e
	}

	// Build symbol lookup by name.
	symbolByName := make(map[string]*extractor.Symbol)
	for i := range fer.Result.Symbols {
		symbolByName[fer.Result.Symbols[i].Name] = &fer.Result.Symbols[i]
	}

	candidateNames := make(map[string]bool)

	// First pass: directly exported symbols.
	for i := range fer.Result.Symbols {
		sym := &fer.Result.Symbols[i]
		if !isComponentCandidate(sym) {
			continue
		}
		candidates = append(candidates, candidate{symbol: sym, export: exportMap[sym.Name]})
		candidateNames[sym.Name] = true
	}

	// Second pass: exports that reference non-exported symbols
	// (e.g., `const Input = React.forwardRef(...); export { Input }`).
	for i := range fer.Result.Exports {
		e := &fer.Result.Exports[i]
		if !isUppercase(e.Name) || candidateNames[e.Name] {
			continue
		}
		if sym, ok := symbolByName[e.Name]; ok {
			candidates = append(candidates, candidate{symbol: sym, export: e})
			candidateNames[e.Name] = true
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Step B: Parse the file once for AST walking.
	lang := parser.DetectLanguage(fer.FilePath)
	isTSX := parser.IsTSXFile(fer.FilePath)
	tree, err := pm.Parse(fer.SourceCode, lang, isTSX)
	if err != nil {
		return nil
	}
	defer tree.Close()
	root := tree.RootNode()

	// Step C: Classify each candidate.
	var components []DetectedComponent
	for _, c := range candidates {
		comp := classifyCandidate(c.symbol, c.export, root, fer.SourceCode, fer.FilePath, fer.Result.Symbols)
		if comp != nil {
			components = append(components, *comp)
		}
	}

	return components
}

// isComponentCandidate checks if a symbol could be a React component.
func isComponentCandidate(sym *extractor.Symbol) bool {
	if !sym.IsExported {
		return false
	}
	if !isUppercase(sym.Name) {
		return false
	}
	switch sym.Kind {
	case extractor.SymbolKindFunction, extractor.SymbolKindVariable, extractor.SymbolKindClass:
		return true
	}
	return false
}

// isUppercase checks if a string starts with an uppercase letter.
func isUppercase(s string) bool {
	if s == "" {
		return false
	}
	return unicode.IsUpper(rune(s[0]))
}

// classifyCandidate determines if a candidate symbol is a React component
// and what kind it is.
func classifyCandidate(
	sym *extractor.Symbol,
	export *extractor.ExportInfo,
	root *ts.Node,
	source []byte,
	filePath string,
	allSymbols []extractor.Symbol,
) *DetectedComponent {
	var kind ComponentKind
	detected := false

	switch sym.Kind {
	case extractor.SymbolKindFunction:
		// Check if function body contains JSX.
		body := getFunctionBody(root, sym)
		if body != nil && containsJSXNode(body) {
			kind = ComponentKindFunction
			detected = true
		}

	case extractor.SymbolKindVariable:
		// Check the value: arrow function with JSX, forwardRef, or memo.
		val := getVariableValue(root, sym, source)
		if val != nil {
			switch {
			case val.Kind() == "call_expression" && isForwardRefCall(val, source):
				kind = ComponentKindForwardRef
				detected = true
			case val.Kind() == "call_expression" && isMemoCall(val, source):
				kind = ComponentKindMemo
				detected = true
			case val.Kind() == "arrow_function":
				body := val.ChildByFieldName("body")
				if body != nil && containsJSXNode(body) {
					kind = ComponentKindFunction
					detected = true
				}
			case val.Kind() == "parenthesized_expression":
				// Arrow function wrapped in parens: const Card = ({ title }) => (<div>...</div>)
				if containsJSXNode(val) {
					kind = ComponentKindFunction
					detected = true
				}
			}
		}

	case extractor.SymbolKindClass:
		if extendsReactComponent(root, sym, source) {
			kind = ComponentKindClass
			detected = true
		}
	}

	if !detected {
		return nil
	}

	isDefault := false
	if export != nil {
		isDefault = export.ExportType == extractor.ExportTypeDefault
	}

	// Step D: Extract props reference.
	propsTypeName := extractPropsTypeName(root, sym, kind, source)
	var propsRef *PropsRef
	if propsTypeName != "" {
		propsRef = &PropsRef{
			TypeName: propsTypeName,
			Symbol:   matchPropsSymbol(propsTypeName, allSymbols),
		}
	}

	return &DetectedComponent{
		Name:            sym.Name,
		FilePath:        filePath,
		Kind:            kind,
		IsExported:      true,
		IsDefaultExport: isDefault,
		PropsRef:        propsRef,
		Symbol:          sym,
	}
}

// groupCompoundComponents groups components from the same file by shared name prefix.
func groupCompoundComponents(components []DetectedComponent) []CompoundGroup {
	// Group by file path.
	byFile := make(map[string][]*DetectedComponent)
	for i := range components {
		c := &components[i]
		byFile[c.FilePath] = append(byFile[c.FilePath], c)
	}

	var groups []CompoundGroup
	for _, comps := range byFile {
		if len(comps) < 2 {
			continue
		}

		// Sort by name length to find potential parent (shortest name).
		sort.Slice(comps, func(i, j int) bool {
			return len(comps[i].Name) < len(comps[j].Name)
		})

		parent := comps[0]
		prefix := parent.Name

		// Check if all others share the prefix.
		allMatch := true
		var subs []*DetectedComponent
		for _, c := range comps[1:] {
			if strings.HasPrefix(c.Name, prefix) {
				subs = append(subs, c)
			} else {
				allMatch = false
				break
			}
		}

		if allMatch && len(subs) > 0 {
			groups = append(groups, CompoundGroup{
				Parent:        parent,
				SubComponents: subs,
			})
		}
	}

	return groups
}

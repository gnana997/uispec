// Symbol extraction implementation.
package extractor

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
)

// extractSymbols processes symbol query matches into Symbol structs.
//
// This includes:
// - Extracting symbol names and kinds from query captures
// - Building fully qualified names (FQN) by walking scope chain
// - Extracting metadata (visibility, modifiers, parameters, return types)
// - Detecting whether symbol is exported
func (e *Extractor) extractSymbols(matches []queries.QueryMatch, tree *ts.Tree, sourceCode []byte, filePath string, lang parser.Language) []Symbol {
	symbols := make([]Symbol, 0, len(matches))

	for _, match := range matches {
		symbol := e.buildSymbol(match, tree, sourceCode, filePath, lang)
		if symbol != nil {
			symbols = append(symbols, *symbol)
		}
	}

	return symbols
}

// buildSymbol creates a Symbol from query captures.
//
// Steps:
// 1. Extract name from @{prefix}.name capture
// 2. Infer kind from capture prefix
// 3. Find declaration node (entire function/class, not just identifier)
// 4. Extract location from DECLARATION node (captures full function/class body)
// 5. Build FQN by walking scope chain from identifier
// 6. Extract metadata (via metadata.go)
func (e *Extractor) buildSymbol(match queries.QueryMatch, tree *ts.Tree, sourceCode []byte, filePath string, lang parser.Language) *Symbol {
	// Find the name capture (e.g., @function.name, @class.name, etc.)
	nameCapture := e.findNameCapture(match.Captures)
	if nameCapture == nil {
		return nil
	}

	name := nameCapture.Text

	// Infer kind from capture prefix
	// e.g., "function.name" → prefix="function" → kind=SymbolKindFunction
	kind := e.inferSymbolKind(nameCapture.Category)

	// Get identifier node for FQN building
	definitionNode := nameCapture.Node

	// Find the declaration node (entire function/class declaration)
	// This is the parent node that contains the full symbol including body
	declarationNode := e.findDeclarationNode(definitionNode, kind)

	// Extract location from DECLARATION node (not identifier)
	// This captures the entire function/class body for code fetching
	var location Location
	if declarationNode != nil {
		location = e.extractLocation(declarationNode, filePath)
	} else {
		// Fallback to identifier node if declaration not found
		location = e.extractLocation(definitionNode, filePath)
	}

	// Build FQN by walking scope chain from identifier node
	// Using identifier ensures correct scope resolution
	fqn := e.buildFQN(definitionNode, name, sourceCode, lang, kind)

	// Create base symbol
	symbol := &Symbol{
		Name:               name,
		FullyQualifiedName: fqn,
		Kind:               kind,
		Location:           location,
	}

	// Extract metadata (visibility, modifiers, parameters, return types)
	// declarationNode is already found above
	if declarationNode != nil {
		e.extractMetadata(symbol, declarationNode, sourceCode, lang)
	}

	// Detect if exported (language-specific)
	symbol.IsExported = e.isExported(definitionNode, name, sourceCode, lang)

	return symbol
}

// findNameCapture finds the capture with ".name" field in its name.
//
// Tree-sitter queries use capture names like:
// - @function.name
// - @class.name
// - @method.name
//
// We look for the one with Field == "name" to get the symbol's name.
func (e *Extractor) findNameCapture(captures []queries.QueryCapture) *queries.QueryCapture {
	for i := range captures {
		if captures[i].Field == "name" {
			return &captures[i]
		}
	}
	return nil
}

// inferSymbolKind infers SymbolKind from capture category.
//
// The category comes from the capture prefix:
// - "function" → SymbolKindFunction
// - "class" → SymbolKindClass
// - "method" → SymbolKindMethod
// - etc.
func (e *Extractor) inferSymbolKind(category string) SymbolKind {
	switch category {
	case "function", "func":
		return SymbolKindFunction
	case "class":
		return SymbolKindClass
	case "interface":
		return SymbolKindInterface
	case "type":
		return SymbolKindType
	case "variable", "var", "let", "const":
		return SymbolKindVariable
	case "constant":
		return SymbolKindConstant
	case "enum":
		return SymbolKindEnum
	case "method":
		return SymbolKindMethod
	case "property", "field":
		return SymbolKindProperty
	default:
		return SymbolKindVariable // Default fallback
	}
}

// findDeclarationNode finds the parent declaration node that contains metadata.
//
// The query captures give us the identifier (name) node, but metadata like
// visibility, modifiers, parameters, and return types are on the parent
// declaration node (function_declaration, method_definition, etc.).
//
// This walks up the tree to find the appropriate declaration node.
func (e *Extractor) findDeclarationNode(nameNode *ts.Node, kind SymbolKind) *ts.Node {
	// Declaration node types for TypeScript/JavaScript
	declarationTypes := map[string]bool{
		"function_declaration":   true,
		"method_definition":     true,
		"class_declaration":     true,
		"interface_declaration": true,
		"type_alias_declaration": true,
		"lexical_declaration":   true,
		"variable_declaration":  true,
		"function_signature":    true,
		"method_signature":      true,
	}

	// Walk up the tree to find a declaration node
	current := nameNode.Parent()
	maxDepth := 10 // Prevent infinite loops
	depth := 0

	for current != nil && depth < maxDepth {
		nodeType := current.GrammarName()
		if declarationTypes[nodeType] {
			return current
		}
		current = current.Parent()
		depth++
	}

	// If we can't find a declaration node, return the original node
	// Metadata extraction will handle this gracefully
	return nameNode
}

// buildFQN constructs fully qualified name by walking up the scope chain.
//
// FQN format varies by language:
// - TypeScript/JavaScript: "ClassName.methodName" or "moduleName.functionName"
// - Python: "ClassName.method_name" or "module_name.function_name"
// - Go: "ReceiverType.MethodName" or "packageName.FunctionName"
// - Rust: "ImplType::method" or "module::function"
//
// Algorithm:
// 1. Walk up parent chain to find enclosing scopes (classes, impl blocks, etc.)
// 2. Build scope chain from outermost to innermost
// 3. For top-level symbols, prepend module name
// 4. Join with dots (or :: for Rust)
func (e *Extractor) buildFQN(node *ts.Node, name string, sourceCode []byte, lang parser.Language, kind SymbolKind) string {
	scopeChain := []string{}

	// Walk up parent chain to find enclosing scopes
	current := node.Parent()
	for current != nil {
		scopeName := e.extractScopeName(current, sourceCode, lang)
		if scopeName != "" {
			// Prepend to maintain outer → inner order
			scopeChain = append([]string{scopeName}, scopeChain...)
		}
		current = current.Parent()
	}

	// For top-level symbols, consider prefixing with module name
	// (Currently disabled - can be enabled later if needed)
	// needsModulePrefix := len(scopeChain) == 0 && (kind == SymbolKindVariable || kind == SymbolKindFunction)
	// if needsModulePrefix {
	// 	moduleName := e.extractModuleName(filePath)
	// 	scopeChain = append(scopeChain, moduleName)
	// }

	// Append symbol name
	scopeChain = append(scopeChain, name)

	// Join with dot separator
	return strings.Join(scopeChain, ".")
}

// extractScopeName extracts scope name from parent node (class, impl block, namespace, etc.).
//
// Returns empty string if node is not a scope-defining construct.
func (e *Extractor) extractScopeName(node *ts.Node, sourceCode []byte, lang parser.Language) string {
	nodeType := node.GrammarName()

	switch lang {
	case parser.LanguageTypeScript, parser.LanguageJavaScript:
		return e.extractTSScopeName(node, nodeType, sourceCode)
	}

	return ""
}

// extractTSScopeName extracts scope name for TypeScript/JavaScript.
func (e *Extractor) extractTSScopeName(node *ts.Node, nodeType string, sourceCode []byte) string {
	switch nodeType {
	case "class_declaration", "class":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return string(nameNode.Utf8Text(sourceCode))
		}
	case "namespace_declaration", "module_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return string(nameNode.Utf8Text(sourceCode))
		}
	case "interface_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return string(nameNode.Utf8Text(sourceCode))
		}
	}
	return ""
}

// extractLocation converts tree-sitter node position to Location struct.
//
// Tree-sitter uses 0-based positions, but LSP uses 1-based, so we add 1 to line/column.
// Byte offsets are kept as 0-based for direct slicing (sourceCode[start:end]).
//
// Performance: node.StartByte() and node.EndByte() are O(1) operations.
func (e *Extractor) extractLocation(node *ts.Node, filePath string) Location {
	startPos := node.StartPosition()
	endPos := node.EndPosition()

	return Location{
		FilePath:    filePath,
		StartLine:   uint32(startPos.Row + 1),    // Convert to 1-based
		StartColumn: uint32(startPos.Column + 1), // Convert to 1-based
		EndLine:     uint32(endPos.Row + 1),
		EndColumn:   uint32(endPos.Column + 1),
		StartByte:   uint32(node.StartByte()), // 0-indexed byte offset (inclusive)
		EndByte:     uint32(node.EndByte()),   // 0-indexed byte offset (exclusive)
	}
}

// isExported checks if a symbol is exported from its module.
//
// Language-specific rules:
// - TypeScript/JavaScript: Has 'export' keyword
// - Python: Not prefixed with '_' (by convention)
// - Go: Uppercase first letter
// - Rust: Has 'pub' visibility modifier
func (e *Extractor) isExported(node *ts.Node, name string, sourceCode []byte, lang parser.Language) bool {
	switch lang {
	case parser.LanguageTypeScript, parser.LanguageJavaScript:
		// Check if parent or grandparent is export_statement
		parent := node.Parent()
		if parent != nil && (parent.GrammarName() == "export_statement" || parent.GrammarName() == "export_declaration") {
			return true
		}
		grandparent := parent.Parent()
		if grandparent != nil && (grandparent.GrammarName() == "export_statement" || grandparent.GrammarName() == "export_declaration") {
			return true
		}
	}

	return false
}

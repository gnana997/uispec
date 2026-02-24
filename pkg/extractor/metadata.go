// Metadata extraction via AST traversal.
package extractor

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/gnana997/uispec/pkg/parser"
)

// extractMetadata walks AST node to extract metadata.
//
// Metadata includes:
// - Visibility/scope: public, private, protected
// - Modifiers: static, async, readonly, abstract, const, unsafe
// - Parameters: names and types
// - Return type
//
// This is done via AST traversal (not queries) because metadata requires
// examining the node's children and field names.
func (e *Extractor) extractMetadata(symbol *Symbol, node *ts.Node, sourceCode []byte, lang parser.Language) {
	switch lang {
	case parser.LanguageTypeScript, parser.LanguageJavaScript:
		e.extractTSMetadata(symbol, node, sourceCode)
	}
}

// extractTSMetadata extracts TypeScript/JavaScript metadata.
func (e *Extractor) extractTSMetadata(symbol *Symbol, node *ts.Node, sourceCode []byte) {
	modifiers := []string{}

	// Iterate through children to find modifiers
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.GrammarName()
		childText := string(child.Utf8Text(sourceCode))

		// Extract visibility (public, private, protected)
		if childType == "accessibility_modifier" {
			symbol.Scope = childText // "public", "private", "protected"
		}

		// Extract modifiers
		switch childText {
		case "static":
			modifiers = append(modifiers, "static")
		case "async":
			modifiers = append(modifiers, "async")
		case "readonly":
			modifiers = append(modifiers, "readonly")
		case "abstract":
			modifiers = append(modifiers, "abstract")
		case "const":
			modifiers = append(modifiers, "const")
		case "export":
			// Skip, handled in isExported check
		}
	}

	if len(modifiers) > 0 {
		symbol.Modifiers = modifiers
	}

	// Extract parameters
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		params, paramTypes := e.extractTSParameters(paramsNode, sourceCode)
		symbol.Parameters = params
		symbol.ParameterTypes = paramTypes
	}

	// Extract return type
	returnTypeNode := node.ChildByFieldName("return_type")
	if returnTypeNode != nil {
		// TypeScript return_type node contains the ':' and the type
		// We want just the type part
		for i := uint(0); i < returnTypeNode.ChildCount(); i++ {
			child := returnTypeNode.Child(i)
			if child != nil && child.GrammarName() != ":" {
				symbol.ReturnType = string(child.Utf8Text(sourceCode))
				break
			}
		}
	}
}

// extractTSParameters extracts parameter names and types from TypeScript/JavaScript.
func (e *Extractor) extractTSParameters(paramsNode *ts.Node, sourceCode []byte) ([]string, []string) {
	params := []string{}
	paramTypes := []string{}

	for i := uint(0); i < paramsNode.NamedChildCount(); i++ {
		param := paramsNode.NamedChild(i)
		if param == nil {
			continue
		}

		paramType := param.GrammarName()

		// Handle different parameter types
		switch paramType {
		case "required_parameter", "optional_parameter":
			// Get parameter name (pattern field)
			nameNode := param.ChildByFieldName("pattern")
			if nameNode == nil {
				nameNode = param.ChildByFieldName("name")
			}
			if nameNode != nil {
				paramName := string(nameNode.Utf8Text(sourceCode))
				params = append(params, paramName)

				// Get parameter type if available
				typeNode := param.ChildByFieldName("type")
				if typeNode != nil {
					// Type node contains ': type', extract just the type
					typeText := string(typeNode.Utf8Text(sourceCode))
					typeText = strings.TrimPrefix(typeText, ":")
					typeText = strings.TrimSpace(typeText)
					paramTypes = append(paramTypes, typeText)
				} else {
					paramTypes = append(paramTypes, "")
				}
			}
		}
	}

	return params, paramTypes
}


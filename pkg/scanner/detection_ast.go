package scanner

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/gnana997/uispec/pkg/extractor"
)

// containsJSXNode recursively checks if any descendant is a JSX element.
func containsJSXNode(node *ts.Node) bool {
	if node == nil {
		return false
	}
	kind := node.Kind()
	if kind == "jsx_element" || kind == "jsx_self_closing_element" || kind == "jsx_fragment" {
		return true
	}
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		if containsJSXNode(node.Child(i)) {
			return true
		}
	}
	return false
}

// findNodeAtByteRange locates the AST node covering the given byte range.
// Returns the deepest node whose range contains [startByte, endByte).
func findNodeAtByteRange(root *ts.Node, startByte, endByte uint32) *ts.Node {
	if root == nil {
		return nil
	}
	if uint32(root.StartByte()) > startByte || uint32(root.EndByte()) < endByte {
		return nil
	}
	// Try to find a more specific child.
	for i := uint(0); i < uint(root.ChildCount()); i++ {
		child := root.Child(i)
		if uint32(child.StartByte()) <= startByte && uint32(child.EndByte()) >= endByte {
			return findNodeAtByteRange(child, startByte, endByte)
		}
	}
	return root
}

// getCallExpressionCallee returns the callee name from a call_expression node.
// Handles "forwardRef(...)" → "forwardRef" and "React.forwardRef(...)" → "React.forwardRef".
func getCallExpressionCallee(node *ts.Node, source []byte) string {
	if node == nil || node.Kind() != "call_expression" {
		return ""
	}
	fn := node.ChildByFieldName("function")
	if fn == nil {
		return ""
	}
	return fn.Utf8Text(source)
}

// isForwardRefCall checks if a call_expression's callee is forwardRef or React.forwardRef.
func isForwardRefCall(node *ts.Node, source []byte) bool {
	callee := getCallExpressionCallee(node, source)
	return callee == "forwardRef" || callee == "React.forwardRef"
}

// isMemoCall checks if a call_expression's callee is memo or React.memo.
func isMemoCall(node *ts.Node, source []byte) bool {
	callee := getCallExpressionCallee(node, source)
	return callee == "memo" || callee == "React.memo"
}

// getVariableValue returns the "value" child of a variable_declarator node.
// Given a symbol location, finds the variable_declarator and returns its value.
func getVariableValue(root *ts.Node, sym *extractor.Symbol, source []byte) *ts.Node {
	decl := findNodeAtByteRange(root, sym.Location.StartByte, sym.Location.EndByte)
	if decl == nil {
		return nil
	}
	// Walk up to find variable_declarator if needed.
	for decl != nil {
		if decl.Kind() == "variable_declarator" {
			return decl.ChildByFieldName("value")
		}
		if decl.Kind() == "lexical_declaration" {
			// Look for the variable_declarator child matching by name.
			for i := uint(0); i < uint(decl.ChildCount()); i++ {
				child := decl.Child(i)
				if child.Kind() == "variable_declarator" {
					name := child.ChildByFieldName("name")
					if name != nil && name.Utf8Text(source) == sym.Name {
						return child.ChildByFieldName("value")
					}
				}
			}
		}
		decl = decl.Parent()
	}
	return nil
}

// getFunctionBody returns the body of a function_declaration or arrow_function.
func getFunctionBody(root *ts.Node, sym *extractor.Symbol) *ts.Node {
	decl := findNodeAtByteRange(root, sym.Location.StartByte, sym.Location.EndByte)
	if decl == nil {
		return nil
	}
	// Walk up to find the function node.
	for decl != nil {
		kind := decl.Kind()
		if kind == "function_declaration" || kind == "arrow_function" || kind == "function" {
			return decl.ChildByFieldName("body")
		}
		// For exported function declarations wrapped in export_statement.
		if kind == "export_statement" {
			for i := uint(0); i < uint(decl.ChildCount()); i++ {
				child := decl.Child(i)
				if child.Kind() == "function_declaration" {
					return child.ChildByFieldName("body")
				}
			}
		}
		decl = decl.Parent()
	}
	return nil
}

// extendsReactComponent checks if a class has a heritage clause extending
// React.Component, React.PureComponent, or Component.
func extendsReactComponent(root *ts.Node, sym *extractor.Symbol, source []byte) bool {
	decl := findNodeAtByteRange(root, sym.Location.StartByte, sym.Location.EndByte)
	if decl == nil {
		return false
	}
	// Walk up to find class_declaration.
	for decl != nil {
		if decl.Kind() == "class_declaration" || decl.Kind() == "class" {
			break
		}
		if decl.Kind() == "export_statement" {
			for i := uint(0); i < uint(decl.ChildCount()); i++ {
				child := decl.Child(i)
				if child.Kind() == "class_declaration" {
					decl = child
					break
				}
			}
			if decl.Kind() == "class_declaration" {
				break
			}
		}
		decl = decl.Parent()
	}
	if decl == nil {
		return false
	}

	// Look for class_heritage in children.
	for i := uint(0); i < uint(decl.ChildCount()); i++ {
		child := decl.Child(i)
		if child.Kind() == "class_heritage" {
			text := child.Utf8Text(source)
			return strings.Contains(text, "Component") || strings.Contains(text, "PureComponent")
		}
	}
	return false
}

// extractPropsTypeName extracts the props type name from a component declaration.
func extractPropsTypeName(root *ts.Node, sym *extractor.Symbol, kind ComponentKind, source []byte) string {
	switch kind {
	case ComponentKindFunction:
		return extractFunctionPropsType(root, sym, source)
	case ComponentKindForwardRef:
		return extractForwardRefPropsType(root, sym, source)
	case ComponentKindMemo:
		return extractMemoPropsType(root, sym, source)
	case ComponentKindClass:
		return extractClassPropsType(root, sym, source)
	}
	return ""
}

// extractFunctionPropsType extracts props type from function/arrow parameters.
// Handles: function Button(props: ButtonProps) and function Button({ variant }: ButtonProps)
func extractFunctionPropsType(root *ts.Node, sym *extractor.Symbol, source []byte) string {
	// For function symbols, find the function node.
	body := getFunctionBody(root, sym)
	if body == nil {
		// Try variable → arrow function path.
		val := getVariableValue(root, sym, source)
		if val != nil && val.Kind() == "arrow_function" {
			return extractParamsType(val, source)
		}
		return ""
	}
	// Get the function node (parent of body).
	fn := body.Parent()
	if fn == nil {
		return ""
	}
	return extractParamsType(fn, source)
}

// extractParamsType extracts the type annotation from the first parameter of a function.
func extractParamsType(fnNode *ts.Node, source []byte) string {
	params := fnNode.ChildByFieldName("parameters")
	if params == nil {
		return ""
	}
	// Find the first required_parameter or optional_parameter.
	for i := uint(0); i < uint(params.ChildCount()); i++ {
		child := params.Child(i)
		kind := child.Kind()
		if kind == "required_parameter" || kind == "optional_parameter" {
			return extractTypeFromParam(child, source)
		}
	}
	return ""
}

// extractTypeFromParam extracts the type name from a parameter node.
// Handles: (props: ButtonProps), ({ variant }: ButtonProps)
func extractTypeFromParam(param *ts.Node, source []byte) string {
	typeAnno := param.ChildByFieldName("type")
	if typeAnno == nil {
		return ""
	}
	// type_annotation contains the actual type.
	// Walk children to find the type identifier.
	for i := uint(0); i < uint(typeAnno.ChildCount()); i++ {
		child := typeAnno.Child(i)
		kind := child.Kind()
		if kind == "type_identifier" {
			return child.Utf8Text(source)
		}
		// For intersection/union types, take the first type_identifier.
		if kind == "intersection_type" || kind == "union_type" {
			return findFirstTypeIdentifier(child, source)
		}
	}
	return ""
}

// findFirstTypeIdentifier recursively finds the first type_identifier in a type node.
func findFirstTypeIdentifier(node *ts.Node, source []byte) string {
	if node == nil {
		return ""
	}
	if node.Kind() == "type_identifier" {
		return node.Utf8Text(source)
	}
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		if result := findFirstTypeIdentifier(node.Child(i), source); result != "" {
			return result
		}
	}
	return ""
}

// extractForwardRefPropsType extracts props type from forwardRef<Element, Props>(...).
func extractForwardRefPropsType(root *ts.Node, sym *extractor.Symbol, source []byte) string {
	val := getVariableValue(root, sym, source)
	if val == nil || val.Kind() != "call_expression" {
		return ""
	}
	// Look for type_arguments on the call expression.
	for i := uint(0); i < uint(val.ChildCount()); i++ {
		child := val.Child(i)
		if child.Kind() == "type_arguments" {
			// Second type argument is the props type.
			typeIdx := 0
			for j := uint(0); j < uint(child.ChildCount()); j++ {
				typeChild := child.Child(j)
				if typeChild.Kind() == "type_identifier" || typeChild.Kind() == "generic_type" {
					typeIdx++
					if typeIdx == 2 {
						if typeChild.Kind() == "type_identifier" {
							return typeChild.Utf8Text(source)
						}
						return findFirstTypeIdentifier(typeChild, source)
					}
				}
			}
		}
	}
	// Fallback: check inner function parameters.
	args := val.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}
	for i := uint(0); i < uint(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Kind() == "arrow_function" || child.Kind() == "function_expression" {
			return extractParamsType(child, source)
		}
	}
	return ""
}

// extractMemoPropsType extracts props type from the inner function of React.memo.
func extractMemoPropsType(root *ts.Node, sym *extractor.Symbol, source []byte) string {
	val := getVariableValue(root, sym, source)
	if val == nil || val.Kind() != "call_expression" {
		return ""
	}
	args := val.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}
	for i := uint(0); i < uint(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Kind() == "arrow_function" || child.Kind() == "function_expression" {
			return extractParamsType(child, source)
		}
	}
	return ""
}

// extractClassPropsType extracts props type from class heritage: extends Component<Props>.
func extractClassPropsType(root *ts.Node, sym *extractor.Symbol, source []byte) string {
	decl := findNodeAtByteRange(root, sym.Location.StartByte, sym.Location.EndByte)
	if decl == nil {
		return ""
	}
	for decl != nil {
		if decl.Kind() == "class_declaration" || decl.Kind() == "class" {
			break
		}
		if decl.Kind() == "export_statement" {
			for i := uint(0); i < uint(decl.ChildCount()); i++ {
				child := decl.Child(i)
				if child.Kind() == "class_declaration" {
					decl = child
					break
				}
			}
			break
		}
		decl = decl.Parent()
	}
	if decl == nil {
		return ""
	}
	// Find class_heritage → type_arguments → first type.
	for i := uint(0); i < uint(decl.ChildCount()); i++ {
		child := decl.Child(i)
		if child.Kind() == "class_heritage" {
			return findFirstTypeIdentifier(child, source)
		}
	}
	return ""
}

// matchPropsSymbol finds a matching interface/type symbol by name in the same file.
func matchPropsSymbol(typeName string, symbols []extractor.Symbol) *extractor.Symbol {
	for i := range symbols {
		s := &symbols[i]
		if s.Name == typeName && (s.Kind == extractor.SymbolKindInterface || s.Kind == extractor.SymbolKindType) {
			return s
		}
	}
	return nil
}

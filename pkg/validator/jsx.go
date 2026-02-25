package validator

import (
	"unicode"

	ts "github.com/tree-sitter/go-tree-sitter"
)

// JSXUsage represents a single JSX component usage in the code.
type JSXUsage struct {
	ComponentName   string            `json:"component_name"`
	Props           map[string]string `json:"props"`            // prop name → literal value ("" for expressions)
	HasChildren     bool              `json:"has_children"`
	ParentComponent string            `json:"parent_component"` // nearest ancestor component ("" if none)
	Line            int               `json:"line"`             // 1-based
	Column          int               `json:"column"`           // 1-based
}

// ImportInfo represents an import statement extracted from the code.
type ImportInfo struct {
	Source      string   `json:"source"`
	Names       []string `json:"names"`
	DefaultName string   `json:"default_name,omitempty"`
	Line        int      `json:"line"`
}

// JSXExtraction holds all extracted JSX usages and imports from a code string.
type JSXExtraction struct {
	Usages  []JSXUsage
	Imports []ImportInfo
}

// ExtractJSX walks a tree-sitter AST and extracts JSX component usages and imports.
func ExtractJSX(tree *ts.Tree, source []byte) *JSXExtraction {
	result := &JSXExtraction{}
	root := tree.RootNode()

	// Extract imports from top-level import_statement nodes.
	extractImports(root, source, result)

	// Walk the tree to extract JSX component usages.
	var parentStack []string
	walkJSX(root, source, &parentStack, result)

	return result
}

// extractImports extracts import statements from the AST.
func extractImports(node *ts.Node, source []byte, result *JSXExtraction) {
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Kind() != "import_statement" {
			continue
		}

		info := ImportInfo{
			Line: int(child.StartPosition().Row) + 1,
		}

		for j := uint(0); j < uint(child.ChildCount()); j++ {
			part := child.Child(j)
			switch part.Kind() {
			case "string":
				info.Source = extractStringContent(part, source)
			case "import_clause":
				extractImportClause(part, source, &info)
			}
		}

		if info.Source != "" {
			result.Imports = append(result.Imports, info)
		}
	}
}

// extractImportClause processes the import clause (between "import" and "from").
func extractImportClause(node *ts.Node, source []byte, info *ImportInfo) {
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Kind() {
		case "identifier":
			// Default import: import Button from "..."
			info.DefaultName = child.Utf8Text(source)
		case "named_imports":
			extractNamedImports(child, source, info)
		}
	}
}

// extractNamedImports processes { Button, Dialog } in an import statement.
func extractNamedImports(node *ts.Node, source []byte, info *ImportInfo) {
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Kind() == "import_specifier" {
			// The first identifier child is the imported name.
			for j := uint(0); j < uint(child.ChildCount()); j++ {
				spec := child.Child(j)
				if spec.Kind() == "identifier" {
					info.Names = append(info.Names, spec.Utf8Text(source))
					break
				}
			}
		}
	}
}

// extractStringContent gets the text inside a string node (without quotes).
func extractStringContent(node *ts.Node, source []byte) string {
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Kind() == "string_fragment" {
			return child.Utf8Text(source)
		}
	}
	// Fallback: strip quotes from the full text.
	text := node.Utf8Text(source)
	if len(text) >= 2 {
		return text[1 : len(text)-1]
	}
	return text
}

// walkJSX recursively walks the AST to extract JSX component usages.
func walkJSX(node *ts.Node, source []byte, parentStack *[]string, result *JSXExtraction) {
	kind := node.Kind()

	switch kind {
	case "jsx_element":
		processJSXElement(node, source, parentStack, result)
		return
	case "jsx_self_closing_element":
		processJSXSelfClosing(node, source, parentStack, result)
		return
	}

	// Recurse into all children.
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		walkJSX(node.Child(i), source, parentStack, result)
	}
}

// processJSXElement handles <Component ...>children</Component>.
func processJSXElement(node *ts.Node, source []byte, parentStack *[]string, result *JSXExtraction) {
	var tagName string
	var props map[string]string

	// Get tag name and props from jsx_opening_element.
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Kind() == "jsx_opening_element" {
			tagName, props = extractTagAndProps(child, source)
			break
		}
	}

	// Count component children (not text, not HTML).
	hasChildren := hasJSXChildren(node, source)

	isComponent := isComponentName(tagName)
	parentComponent := currentParent(*parentStack)

	if isComponent {
		result.Usages = append(result.Usages, JSXUsage{
			ComponentName:   tagName,
			Props:           props,
			HasChildren:     hasChildren,
			ParentComponent: parentComponent,
			Line:            int(node.StartPosition().Row) + 1,
			Column:          int(node.StartPosition().Column) + 1,
		})
	}

	// Push this component (or HTML tag) as parent for children.
	if isComponent {
		*parentStack = append(*parentStack, tagName)
	}

	// Recurse into children of this element (skip opening/closing tags).
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		k := child.Kind()
		if k != "jsx_opening_element" && k != "jsx_closing_element" {
			walkJSX(child, source, parentStack, result)
		}
	}

	if isComponent {
		*parentStack = (*parentStack)[:len(*parentStack)-1]
	}
}

// processJSXSelfClosing handles <Component ... />.
func processJSXSelfClosing(node *ts.Node, source []byte, parentStack *[]string, result *JSXExtraction) {
	tagName, props := extractTagAndProps(node, source)

	if isComponentName(tagName) {
		result.Usages = append(result.Usages, JSXUsage{
			ComponentName:   tagName,
			Props:           props,
			HasChildren:     false,
			ParentComponent: currentParent(*parentStack),
			Line:            int(node.StartPosition().Row) + 1,
			Column:          int(node.StartPosition().Column) + 1,
		})
	}
}

// extractTagAndProps gets the tag name and props from an opening element or self-closing element.
func extractTagAndProps(node *ts.Node, source []byte) (string, map[string]string) {
	var tagName string
	props := make(map[string]string)

	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Kind() {
		case "identifier", "member_expression", "nested_identifier":
			if tagName == "" {
				tagName = child.Utf8Text(source)
			}
		case "jsx_attribute":
			name, value := extractAttribute(child, source)
			if name != "" {
				props[name] = value
			}
		case "jsx_expression":
			// Spread props: {...props}
			// We record it as a special prop.
			text := child.Utf8Text(source)
			if len(text) > 2 && text[1] == '.' && text[2] == '.' {
				props["...spread"] = ""
			}
		}
	}

	return tagName, props
}

// extractAttribute gets the name and value from a jsx_attribute node.
func extractAttribute(node *ts.Node, source []byte) (string, string) {
	var name, value string

	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Kind() {
		case "property_identifier":
			name = child.Utf8Text(source)
		case "string":
			value = extractStringContent(child, source)
		case "jsx_expression":
			// Expression value like {() => {}} or {myVar}.
			value = ""
		}
	}

	// Boolean prop (no value): <Button asChild>
	if name != "" && value == "" {
		for i := uint(0); i < uint(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Kind() == "jsx_expression" || child.Kind() == "string" {
				return name, value
			}
		}
		// Truly no value — boolean shorthand.
		value = "true"
	}

	return name, value
}

// hasJSXChildren checks if a jsx_element has any meaningful children (components or text).
func hasJSXChildren(node *ts.Node, source []byte) bool {
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		k := child.Kind()
		if k == "jsx_element" || k == "jsx_self_closing_element" || k == "jsx_expression" {
			return true
		}
		if k == "jsx_text" {
			text := child.Utf8Text(source)
			// Check if there's actual text content (not just whitespace).
			for _, r := range text {
				if !unicode.IsSpace(r) {
					return true
				}
			}
		}
	}
	return false
}

// isComponentName returns true if the tag name starts with an uppercase letter (React component convention).
func isComponentName(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}

// currentParent returns the current parent component name from the stack, or "".
func currentParent(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return stack[len(stack)-1]
}

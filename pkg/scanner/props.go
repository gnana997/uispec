package scanner

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/parser"
)

// ExtractAllProps runs Phase 4 prop extraction for each detected component.
// Returns a map from component name to its extracted props.
func ExtractAllProps(
	components []DetectedComponent,
	resultsByFile map[string]*FileExtractionResult,
	pm *parser.ParserManager,
) map[string]*PropExtractionResult {
	propsMap := make(map[string]*PropExtractionResult)

	// Group components by file path to parse each file once.
	type fileComponents struct {
		fer        *FileExtractionResult
		components []DetectedComponent
	}
	byFile := make(map[string]*fileComponents)
	for _, comp := range components {
		fer, ok := resultsByFile[comp.FilePath]
		if !ok {
			continue
		}
		fc, ok := byFile[comp.FilePath]
		if !ok {
			fc = &fileComponents{fer: fer}
			byFile[comp.FilePath] = fc
		}
		fc.components = append(fc.components, comp)
	}

	// Process each file.
	for _, fc := range byFile {
		lang := parser.DetectLanguage(fc.fer.FilePath)
		isTSX := parser.IsTSXFile(fc.fer.FilePath)
		tree, err := pm.Parse(fc.fer.SourceCode, lang, isTSX)
		if err != nil {
			continue
		}
		root := tree.RootNode()

		// Extract CVA variants once per file, keyed by variable name.
		cvaSets := extractCVAVariants(root, fc.fer.SourceCode)

		for _, comp := range fc.components {
			result := extractComponentProps(comp, root, fc.fer.SourceCode)

			// Selectively merge CVA variants: only merge if the component
			// references the CVA variable via VariantProps<typeof X>.
			if len(cvaSets) > 0 {
				cvaRef := findCVAReference(comp, root, fc.fer.SourceCode)
				if cvaRef != "" {
					// Match by VariantProps<typeof X> reference.
					for _, cvaSet := range cvaSets {
						if cvaSet.VariableName == cvaRef {
							result.Props = mergeCVAProps(result.Props, cvaSet.Props)
							break
						}
					}
				} else if len(cvaSets) == 1 && len(fc.components) == 1 {
					// Fallback: single cva() + single component in file.
					result.Props = mergeCVAProps(result.Props, cvaSets[0].Props)
				}
			}

			propsMap[comp.Name] = result
		}

		tree.Close()
	}

	return propsMap
}

// extractComponentProps extracts props for a single component from a parsed AST.
func extractComponentProps(
	comp DetectedComponent,
	root *ts.Node,
	source []byte,
) *PropExtractionResult {
	result := &PropExtractionResult{
		ComponentName: comp.Name,
		FilePath:      comp.FilePath,
	}

	if comp.PropsRef != nil && comp.PropsRef.Symbol != nil {
		// Primary path: extract from interface/type declaration.
		sym := comp.PropsRef.Symbol
		node := findNodeAtByteRange(root, sym.Location.StartByte, sym.Location.EndByte)
		if node != nil {
			decl := findDeclaration(node)
			if decl != nil {
				var props []ExtractedProp
				switch decl.Kind() {
				case "interface_declaration":
					props = extractPropsFromInterfaceDecl(decl, source)
				case "type_alias_declaration":
					props = extractPropsFromTypeAlias(decl, source)
				}

				// Extract destructuring defaults from the component function.
				defaults := extractDefaults(root, comp.Symbol, comp.Kind, source)
				for i := range props {
					if def, ok := defaults[props[i].Name]; ok {
						props[i].Default = def
					}
				}

				result.Props = props
			}
		}
	} else {
		// Fallback: extract from destructured params + inline type annotation.
		// Handles React.ComponentProps<typeof X>, React.ComponentProps<"input">, etc.
		fnNode := findComponentFunctionNode(comp, root, source)
		if fnNode != nil {
			result.Props = extractPropsFromDestructuredParams(fnNode, source)
		}
	}

	return result
}

// findDeclaration walks up from a node to find the enclosing declaration.
func findDeclaration(node *ts.Node) *ts.Node {
	for node != nil {
		kind := node.Kind()
		if kind == "interface_declaration" || kind == "type_alias_declaration" {
			return node
		}
		// For export_statement wrapping the declaration.
		if kind == "export_statement" {
			for i := uint(0); i < uint(node.ChildCount()); i++ {
				child := node.Child(i)
				ck := child.Kind()
				if ck == "interface_declaration" || ck == "type_alias_declaration" {
					return child
				}
			}
		}
		node = node.Parent()
	}
	return nil
}

// extractPropsFromInterfaceDecl extracts props from an interface_declaration.
func extractPropsFromInterfaceDecl(decl *ts.Node, source []byte) []ExtractedProp {
	// Find interface_body or object_type child.
	body := findChildByKind(decl, "interface_body")
	if body == nil {
		body = findChildByKind(decl, "object_type")
	}
	if body == nil {
		return nil
	}
	return extractPropsFromBody(body, source)
}

// extractPropsFromTypeAlias extracts props from a type_alias_declaration.
func extractPropsFromTypeAlias(decl *ts.Node, source []byte) []ExtractedProp {
	// The value child is the type expression.
	value := decl.ChildByFieldName("value")
	if value == nil {
		return nil
	}
	// If it's an object_type directly.
	if value.Kind() == "object_type" {
		return extractPropsFromBody(value, source)
	}
	// If it's an intersection_type, look for the object_type part.
	if value.Kind() == "intersection_type" {
		for i := uint(0); i < uint(value.ChildCount()); i++ {
			child := value.Child(i)
			if child.Kind() == "object_type" {
				return extractPropsFromBody(child, source)
			}
		}
	}
	return nil
}

// extractPropsFromBody extracts props from an interface_body or object_type node.
func extractPropsFromBody(body *ts.Node, source []byte) []ExtractedProp {
	var props []ExtractedProp

	for i := uint(0); i < uint(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Kind() != "property_signature" {
			continue
		}

		prop := extractPropFromSignature(child, source)
		if prop != nil {
			// Extract JSDoc from the preceding comment.
			desc, deprecated := extractJSDocForProp(body, i, source)
			if desc != "" {
				prop.Description = desc
			}
			if deprecated {
				prop.Deprecated = true
			}
			props = append(props, *prop)
		}
	}

	return props
}

// extractPropFromSignature extracts a single prop from a property_signature node.
func extractPropFromSignature(sig *ts.Node, source []byte) *ExtractedProp {
	// Get prop name.
	nameNode := sig.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Utf8Text(source)

	// Check if optional (has "?" child).
	optional := false
	for i := uint(0); i < uint(sig.ChildCount()); i++ {
		child := sig.Child(i)
		if child.Kind() == "?" {
			optional = true
			break
		}
	}

	// Get type annotation.
	typeName := ""
	var allowedValues []string
	typeAnno := sig.ChildByFieldName("type")
	if typeAnno != nil {
		// type_annotation wraps the actual type.
		typeName, allowedValues = resolveTypeAnnotation(typeAnno, source)
	}

	return &ExtractedProp{
		Name:          name,
		Type:          typeName,
		Required:      !optional,
		AllowedValues: allowedValues,
	}
}

// resolveTypeAnnotation extracts the type from a type_annotation node.
func resolveTypeAnnotation(typeAnno *ts.Node, source []byte) (string, []string) {
	// type_annotation has children like ":" and the actual type node.
	for i := uint(0); i < uint(typeAnno.ChildCount()); i++ {
		child := typeAnno.Child(i)
		kind := child.Kind()
		if kind == ":" {
			continue
		}
		return resolveType(child, source)
	}
	return "", nil
}

// resolveType resolves a type AST node to a simplified type string and allowed values.
func resolveType(node *ts.Node, source []byte) (string, []string) {
	if node == nil {
		return "", nil
	}

	kind := node.Kind()

	switch kind {
	case "predefined_type":
		// "string", "number", "boolean", "any", "void", "never", "undefined", "null"
		return node.Utf8Text(source), nil

	case "type_identifier":
		return node.Utf8Text(source), nil

	case "literal_type":
		// A single literal like "default" or true or 42.
		text := node.Utf8Text(source)
		return inferLiteralType(text), nil

	case "union_type":
		return resolveUnionType(node, source)

	case "generic_type":
		return resolveGenericType(node, source), nil

	case "function_type":
		return "function", nil

	case "array_type":
		return "array", nil

	case "parenthesized_type":
		// Unwrap: (Type) → Type
		for i := uint(0); i < uint(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Kind() != "(" && child.Kind() != ")" {
				return resolveType(child, source)
			}
		}
		return node.Utf8Text(source), nil

	case "member_expression", "nested_type_identifier":
		// React.ReactNode, React.ChangeEvent<...>
		return resolveQualifiedType(node, source), nil

	case "object_type":
		return "object", nil

	case "tuple_type":
		return "tuple", nil

	case "indexed_access_type":
		return node.Utf8Text(source), nil

	default:
		// Fallback: return raw text.
		return node.Utf8Text(source), nil
	}
}

// resolveUnionType handles union types like "default" | "destructive" | "outline".
// Tree-sitter parses multi-member unions as left-recursive binary trees, so we flatten them.
func resolveUnionType(node *ts.Node, source []byte) (string, []string) {
	members := flattenUnionMembers(node, source)

	var literals []string
	allStringLiterals := true
	allLiterals := true

	for _, m := range members {
		if m.kind == "literal_type" {
			if isStringLiteral(m.text) {
				literals = append(literals, unquoteString(m.text))
			} else {
				allStringLiterals = false
				literals = append(literals, m.text)
			}
		} else {
			allLiterals = false
			allStringLiterals = false
		}
	}

	if allStringLiterals && len(literals) > 0 {
		return "string", literals
	}
	if allLiterals && len(literals) > 0 {
		return "union", literals
	}

	// Mixed union — return raw text.
	return node.Utf8Text(source), nil
}

type unionMember struct {
	kind string
	text string
}

// flattenUnionMembers recursively flattens a binary union tree into its leaf members.
func flattenUnionMembers(node *ts.Node, source []byte) []unionMember {
	if node == nil {
		return nil
	}
	if node.Kind() != "union_type" {
		return []unionMember{{kind: node.Kind(), text: node.Utf8Text(source)}}
	}
	var members []unionMember
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Kind() == "|" {
			continue
		}
		members = append(members, flattenUnionMembers(child, source)...)
	}
	return members
}

// resolveGenericType handles React.ReactNode, HTMLDivElement, etc.
func resolveGenericType(node *ts.Node, source []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		name := nameNode.Utf8Text(source)
		// Simplify common React types.
		switch name {
		case "ReactNode", "React.ReactNode":
			return "ReactNode"
		case "ReactElement", "React.ReactElement":
			return "ReactElement"
		}
		return name
	}
	return node.Utf8Text(source)
}

// resolveQualifiedType handles member expressions like React.ReactNode.
func resolveQualifiedType(node *ts.Node, source []byte) string {
	text := node.Utf8Text(source)
	// Simplify React.X → X
	if strings.HasPrefix(text, "React.") {
		return strings.TrimPrefix(text, "React.")
	}
	return text
}

// extractDefaults extracts destructuring default values from a component function.
func extractDefaults(root *ts.Node, compSym *extractor.Symbol, kind ComponentKind, source []byte) map[string]string {
	defaults := make(map[string]string)

	if compSym == nil {
		return defaults
	}

	// Find the function node.
	var fnNode *ts.Node

	switch kind {
	case ComponentKindFunction:
		body := getFunctionBody(root, compSym)
		if body != nil {
			fnNode = body.Parent()
		}
		if fnNode == nil {
			// Try arrow function via variable value.
			val := getVariableValue(root, compSym, source)
			if val != nil && val.Kind() == "arrow_function" {
				fnNode = val
			}
		}
	case ComponentKindForwardRef, ComponentKindMemo:
		// Find the inner function inside forwardRef/memo call.
		val := getVariableValue(root, compSym, source)
		if val != nil && val.Kind() == "call_expression" {
			args := val.ChildByFieldName("arguments")
			if args != nil {
				for i := uint(0); i < uint(args.ChildCount()); i++ {
					child := args.Child(i)
					if child.Kind() == "arrow_function" || child.Kind() == "function_expression" {
						fnNode = child
						break
					}
				}
			}
		}
	}

	if fnNode == nil {
		return defaults
	}

	// Get parameters.
	params := fnNode.ChildByFieldName("parameters")
	if params == nil {
		return defaults
	}

	// Find the first parameter.
	for i := uint(0); i < uint(params.ChildCount()); i++ {
		child := params.Child(i)
		if child.Kind() == "required_parameter" || child.Kind() == "optional_parameter" {
			extractDefaultsFromParam(child, source, defaults)
			break
		}
	}

	return defaults
}

// extractDefaultsFromParam extracts defaults from a destructuring parameter.
func extractDefaultsFromParam(param *ts.Node, source []byte, defaults map[string]string) {
	// Look for the pattern child (object_pattern for destructuring).
	pattern := param.ChildByFieldName("pattern")
	if pattern == nil {
		// Try direct child.
		for i := uint(0); i < uint(param.ChildCount()); i++ {
			child := param.Child(i)
			if child.Kind() == "object_pattern" {
				pattern = child
				break
			}
		}
	}

	if pattern == nil || pattern.Kind() != "object_pattern" {
		return
	}

	// Walk object_pattern children for default value assignments.
	// Tree-sitter uses "object_assignment_pattern" for `{ variant = "default" }`.
	for i := uint(0); i < uint(pattern.ChildCount()); i++ {
		child := pattern.Child(i)

		switch child.Kind() {
		case "pair_pattern":
			// { key: localName = defaultValue } — less common.
			right := child.ChildByFieldName("value")
			if right != nil && (right.Kind() == "assignment_pattern" || right.Kind() == "object_assignment_pattern") {
				extractAssignmentDefault(right, source, defaults)
			}
		case "shorthand_property_identifier_pattern":
			// { variant } — no default, skip.
			continue
		case "assignment_pattern", "object_assignment_pattern":
			// { variant = "default" }
			extractAssignmentDefault(child, source, defaults)
		}
	}
}

// extractAssignmentDefault extracts the name and default value from an assignment_pattern.
func extractAssignmentDefault(node *ts.Node, source []byte, defaults map[string]string) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil || right == nil {
		return
	}

	name := left.Utf8Text(source)
	value := right.Utf8Text(source)

	// Strip quotes from string literals.
	if isStringLiteral(value) {
		value = unquoteString(value)
	}

	defaults[name] = value
}

// extractPropsFromDestructuredParams extracts props from a function's destructured
// parameters when no explicit interface/type declaration is available.
// This handles patterns like: function Input({ className, type, ...props }: React.ComponentProps<"input">)
// It also parses inline intersection object types for full type info.
func extractPropsFromDestructuredParams(fnNode *ts.Node, source []byte) []ExtractedProp {
	params := fnNode.ChildByFieldName("parameters")
	if params == nil {
		return nil
	}

	// Find the first parameter.
	var firstParam *ts.Node
	for i := uint(0); i < uint(params.ChildCount()); i++ {
		child := params.Child(i)
		if child.Kind() == "required_parameter" || child.Kind() == "optional_parameter" {
			firstParam = child
			break
		}
	}
	if firstParam == nil {
		return nil
	}

	// Step 1: Extract prop names from destructured object_pattern.
	pattern := firstParam.ChildByFieldName("pattern")
	if pattern == nil {
		for i := uint(0); i < uint(firstParam.ChildCount()); i++ {
			child := firstParam.Child(i)
			if child.Kind() == "object_pattern" {
				pattern = child
				break
			}
		}
	}
	if pattern == nil || pattern.Kind() != "object_pattern" {
		return nil
	}

	// Collect destructured prop names and defaults.
	type destructuredProp struct {
		name         string
		defaultValue string
	}
	var destructured []destructuredProp

	for i := uint(0); i < uint(pattern.ChildCount()); i++ {
		child := pattern.Child(i)
		switch child.Kind() {
		case "shorthand_property_identifier_pattern":
			// { className } — prop name, no default.
			destructured = append(destructured, destructuredProp{
				name: child.Utf8Text(source),
			})
		case "assignment_pattern", "object_assignment_pattern":
			// { size = "default" } — prop name + default value.
			left := child.ChildByFieldName("left")
			right := child.ChildByFieldName("right")
			if left != nil {
				dp := destructuredProp{name: left.Utf8Text(source)}
				if right != nil {
					val := right.Utf8Text(source)
					if isStringLiteral(val) {
						val = unquoteString(val)
					}
					dp.defaultValue = val
				}
				destructured = append(destructured, dp)
			}
		case "pair_pattern":
			// { key: localName = defaultValue } — use key as prop name.
			key := child.ChildByFieldName("key")
			if key != nil {
				dp := destructuredProp{name: key.Utf8Text(source)}
				value := child.ChildByFieldName("value")
				if value != nil && (value.Kind() == "assignment_pattern" || value.Kind() == "object_assignment_pattern") {
					right := value.ChildByFieldName("right")
					if right != nil {
						val := right.Utf8Text(source)
						if isStringLiteral(val) {
							val = unquoteString(val)
						}
						dp.defaultValue = val
					}
				}
				destructured = append(destructured, dp)
			}
			// Skip rest_pattern (...props) — pass-through, not explicit props.
		}
	}

	if len(destructured) == 0 {
		return nil
	}

	// Step 2: Check for inline intersection type containing an object_type.
	// e.g., React.ComponentProps<"button"> & { size?: "sm" | "default" }
	var typedProps map[string]ExtractedProp
	typeAnno := firstParam.ChildByFieldName("type")
	if typeAnno != nil {
		for i := uint(0); i < uint(typeAnno.ChildCount()); i++ {
			child := typeAnno.Child(i)
			if child.Kind() == "intersection_type" {
				// Walk the intersection for object_type children.
				for j := uint(0); j < uint(child.ChildCount()); j++ {
					part := child.Child(j)
					if part.Kind() == "object_type" {
						inlineProps := extractPropsFromBody(part, source)
						if len(inlineProps) > 0 {
							typedProps = make(map[string]ExtractedProp, len(inlineProps))
							for _, p := range inlineProps {
								typedProps[p.Name] = p
							}
						}
					}
				}
			}
		}
	}

	// Step 3: Build final props list. Typed props (from inline intersection) take
	// precedence. Destructured-only props get minimal type info.
	var props []ExtractedProp
	seen := make(map[string]bool, len(destructured))

	for _, dp := range destructured {
		seen[dp.name] = true
		if tp, ok := typedProps[dp.name]; ok {
			// Use typed prop info, but apply destructured default if present.
			if dp.defaultValue != "" && tp.Default == "" {
				tp.Default = dp.defaultValue
			}
			props = append(props, tp)
		} else {
			// Destructured-only: minimal info.
			prop := ExtractedProp{
				Name:     dp.name,
				Type:     "unknown",
				Required: false,
				Default:  dp.defaultValue,
			}
			props = append(props, prop)
		}
	}

	// Add any typed props not present in destructuring (unlikely but possible).
	for name, tp := range typedProps {
		if !seen[name] {
			props = append(props, tp)
		}
	}

	return props
}

// extractJSDocForProp extracts JSDoc comment for a property at the given child index.
func extractJSDocForProp(body *ts.Node, propIndex uint, source []byte) (string, bool) {
	// Look backwards from propIndex for a comment node.
	if propIndex == 0 {
		return "", false
	}

	for i := int(propIndex) - 1; i >= 0; i-- {
		child := body.Child(uint(i))
		if child == nil {
			break
		}
		kind := child.Kind()
		if kind == "comment" {
			return parseJSDoc(child.Utf8Text(source))
		}
		// Stop if we hit another property_signature (comment belongs to it).
		if kind == "property_signature" {
			break
		}
	}
	return "", false
}

// parseJSDoc parses a JSDoc comment and extracts description and deprecated status.
func parseJSDoc(comment string) (string, bool) {
	// Strip /** and */ markers.
	comment = strings.TrimSpace(comment)
	if !strings.HasPrefix(comment, "/**") {
		// Single-line comment: // or /* */
		if strings.HasPrefix(comment, "//") {
			comment = strings.TrimPrefix(comment, "//")
			comment = strings.TrimSpace(comment)
			deprecated := strings.Contains(comment, "@deprecated")
			if deprecated {
				comment = strings.Replace(comment, "@deprecated", "", 1)
				comment = strings.TrimSpace(comment)
			}
			return comment, deprecated
		}
		return "", false
	}

	comment = strings.TrimPrefix(comment, "/**")
	comment = strings.TrimSuffix(comment, "*/")

	deprecated := false
	var descParts []string

	lines := strings.Split(comment, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@deprecated") {
			deprecated = true
			rest := strings.TrimPrefix(line, "@deprecated")
			rest = strings.TrimSpace(rest)
			if rest != "" {
				descParts = append(descParts, rest)
			}
			continue
		}
		if strings.HasPrefix(line, "@") {
			// Skip other tags.
			continue
		}
		descParts = append(descParts, line)
	}

	return strings.Join(descParts, " "), deprecated
}

// findCVAReference finds the CVA variable name referenced by a component via
// VariantProps<typeof X> in its props type (either in extends clause or inline).
func findCVAReference(comp DetectedComponent, root *ts.Node, source []byte) string {
	// Strategy 1: Check the props type declaration (interface/type alias) for
	// VariantProps<typeof X> in extends clause or intersection type.
	if comp.PropsRef != nil && comp.PropsRef.Symbol != nil {
		sym := comp.PropsRef.Symbol
		node := findNodeAtByteRange(root, sym.Location.StartByte, sym.Location.EndByte)
		if node != nil {
			decl := findDeclaration(node)
			if decl != nil {
				if ref := findVariantPropsTypeofInNode(decl, source); ref != "" {
					return ref
				}
			}
		}
	}

	// Strategy 2: Check inline type annotation on the component function's parameter.
	// e.g., function Foo({ ... }: SomeType & VariantProps<typeof fooVariants>) { ... }
	fnNode := findComponentFunctionNode(comp, root, source)
	if fnNode != nil {
		params := fnNode.ChildByFieldName("parameters")
		if params != nil {
			if ref := findVariantPropsTypeofInNode(params, source); ref != "" {
				return ref
			}
		}
	}

	return ""
}

// findVariantPropsTypeofInNode recursively searches a node tree for
// VariantProps<typeof X> and returns X.
func findVariantPropsTypeofInNode(node *ts.Node, source []byte) string {
	if node == nil {
		return ""
	}

	// Check if this is generic_type with name "VariantProps".
	if node.Kind() == "generic_type" {
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil && nameNode.Utf8Text(source) == "VariantProps" {
			// Find type_arguments → type_query → identifier.
			typeArgs := findChildByKind(node, "type_arguments")
			if typeArgs != nil {
				for i := uint(0); i < uint(typeArgs.ChildCount()); i++ {
					child := typeArgs.Child(i)
					if child.Kind() == "type_query" {
						// type_query has "typeof" keyword and identifier.
						for j := uint(0); j < uint(child.ChildCount()); j++ {
							qChild := child.Child(j)
							if qChild.Kind() == "identifier" {
								return qChild.Utf8Text(source)
							}
						}
					}
				}
			}
		}
	}

	// Recurse into children.
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		if ref := findVariantPropsTypeofInNode(node.Child(i), source); ref != "" {
			return ref
		}
	}
	return ""
}

// findComponentFunctionNode finds the function/arrow node for a component.
func findComponentFunctionNode(comp DetectedComponent, root *ts.Node, source []byte) *ts.Node {
	if comp.Symbol == nil {
		return nil
	}

	switch comp.Kind {
	case ComponentKindFunction:
		body := getFunctionBody(root, comp.Symbol)
		if body != nil {
			return body.Parent()
		}
		val := getVariableValue(root, comp.Symbol, source)
		if val != nil && val.Kind() == "arrow_function" {
			return val
		}
	case ComponentKindForwardRef, ComponentKindMemo:
		val := getVariableValue(root, comp.Symbol, source)
		if val != nil && val.Kind() == "call_expression" {
			args := val.ChildByFieldName("arguments")
			if args != nil {
				for i := uint(0); i < uint(args.ChildCount()); i++ {
					child := args.Child(i)
					if child.Kind() == "arrow_function" || child.Kind() == "function_expression" {
						return child
					}
				}
			}
		}
	}
	return nil
}

// mergeCVAProps merges CVA-extracted props into interface-extracted props.
func mergeCVAProps(interfaceProps []ExtractedProp, cvaProps []ExtractedProp) []ExtractedProp {
	// Build lookup from interface props.
	byName := make(map[string]int, len(interfaceProps))
	for i, p := range interfaceProps {
		byName[p.Name] = i
	}

	for _, cvaProp := range cvaProps {
		if idx, ok := byName[cvaProp.Name]; ok {
			// Merge: add allowed values and default from CVA.
			if len(cvaProp.AllowedValues) > 0 && len(interfaceProps[idx].AllowedValues) == 0 {
				interfaceProps[idx].AllowedValues = cvaProp.AllowedValues
			}
			if cvaProp.Default != "" && interfaceProps[idx].Default == "" {
				interfaceProps[idx].Default = cvaProp.Default
			}
		} else {
			// New prop only from CVA.
			interfaceProps = append(interfaceProps, cvaProp)
		}
	}

	return interfaceProps
}

// Helper functions.

func findChildByKind(node *ts.Node, kind string) *ts.Node {
	for i := uint(0); i < uint(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Kind() == kind {
			return child
		}
	}
	return nil
}

func isStringLiteral(s string) bool {
	return (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) ||
		(strings.HasPrefix(s, "`") && strings.HasSuffix(s, "`"))
}

func unquoteString(s string) string {
	if len(s) >= 2 {
		return s[1 : len(s)-1]
	}
	return s
}

func inferLiteralType(text string) string {
	if isStringLiteral(text) {
		return "string"
	}
	if text == "true" || text == "false" {
		return "boolean"
	}
	// Could be a number.
	if len(text) > 0 && (text[0] >= '0' && text[0] <= '9' || text[0] == '-') {
		return "number"
	}
	return text
}

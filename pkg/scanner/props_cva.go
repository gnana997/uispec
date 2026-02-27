package scanner

import (
	ts "github.com/tree-sitter/go-tree-sitter"
)

// extractCVAVariants scans a file's AST for cva() calls and extracts
// variant keys as props with their allowed string values and defaults.
// Each result is associated with the variable name that holds the cva() call.
func extractCVAVariants(root *ts.Node, source []byte) []CVAVariantSet {
	// Find all cva() call expressions in the file.
	cvaNodes := findCVACalls(root, source)
	if len(cvaNodes) == 0 {
		return nil
	}

	var results []CVAVariantSet
	for _, cvaCall := range cvaNodes {
		varName := findCVAVariableName(cvaCall, source)
		variants, defaults := parseCVACall(cvaCall, source)

		var props []ExtractedProp
		for variantName, allowedValues := range variants {
			prop := ExtractedProp{
				Name:          variantName,
				Type:          "string",
				Required:      false,
				AllowedValues: allowedValues,
				Default:       defaults[variantName],
			}
			props = append(props, prop)
		}

		results = append(results, CVAVariantSet{
			VariableName: varName,
			Props:        props,
		})
	}

	return results
}

// findCVAVariableName walks up from a cva() call to find the enclosing
// variable declaration name (e.g., "buttonVariants").
func findCVAVariableName(cvaCall *ts.Node, source []byte) string {
	node := cvaCall.Parent()
	for node != nil {
		kind := node.Kind()
		if kind == "variable_declarator" {
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Utf8Text(source)
			}
		}
		// Stop at statement level to avoid walking too far.
		if kind == "lexical_declaration" || kind == "variable_declaration" ||
			kind == "export_statement" || kind == "program" {
			break
		}
		node = node.Parent()
	}
	return ""
}

// findCVACalls recursively finds all call_expression nodes where callee is "cva".
func findCVACalls(node *ts.Node, source []byte) []*ts.Node {
	if node == nil {
		return nil
	}
	var results []*ts.Node

	if node.Kind() == "call_expression" {
		callee := getCallExpressionCallee(node, source)
		if callee == "cva" {
			results = append(results, node)
		}
	}

	for i := uint(0); i < uint(node.ChildCount()); i++ {
		results = append(results, findCVACalls(node.Child(i), source)...)
	}
	return results
}

// parseCVACall parses a cva() call expression to extract variants and default variants.
// Returns: variants map[variantName][]allowedValues, defaults map[variantName]defaultValue
func parseCVACall(cvaCall *ts.Node, source []byte) (map[string][]string, map[string]string) {
	variants := make(map[string][]string)
	defaults := make(map[string]string)

	args := cvaCall.ChildByFieldName("arguments")
	if args == nil {
		return variants, defaults
	}

	// Find the second argument (config object).
	// Arguments node contains: "(", arg1, ",", arg2, ")"
	configObj := findNthArgument(args, 1) // 0-indexed, so 1 = second arg
	if configObj == nil || configObj.Kind() != "object" {
		return variants, defaults
	}

	// Walk the config object for "variants" and "defaultVariants" keys.
	for i := uint(0); i < uint(configObj.ChildCount()); i++ {
		child := configObj.Child(i)
		if child.Kind() != "pair" {
			continue
		}

		key := child.ChildByFieldName("key")
		value := child.ChildByFieldName("value")
		if key == nil || value == nil {
			continue
		}

		keyText := key.Utf8Text(source)
		switch keyText {
		case "variants":
			parseVariantsObject(value, source, variants)
		case "defaultVariants":
			parseDefaultVariantsObject(value, source, defaults)
		}
	}

	return variants, defaults
}

// findNthArgument finds the nth non-punctuation argument in an arguments node.
func findNthArgument(args *ts.Node, n int) *ts.Node {
	count := 0
	for i := uint(0); i < uint(args.ChildCount()); i++ {
		child := args.Child(i)
		kind := child.Kind()
		if kind == "(" || kind == ")" || kind == "," {
			continue
		}
		if count == n {
			return child
		}
		count++
	}
	return nil
}

// parseVariantsObject parses the "variants" object in a cva() config.
// Each key is a variant name, each value is an object whose keys are allowed values.
func parseVariantsObject(variantsObj *ts.Node, source []byte, variants map[string][]string) {
	if variantsObj.Kind() != "object" {
		return
	}

	for i := uint(0); i < uint(variantsObj.ChildCount()); i++ {
		child := variantsObj.Child(i)
		if child.Kind() != "pair" {
			continue
		}

		key := child.ChildByFieldName("key")
		value := child.ChildByFieldName("value")
		if key == nil || value == nil || value.Kind() != "object" {
			continue
		}

		variantName := key.Utf8Text(source)
		var allowedValues []string

		// Each key in the value object is an allowed value.
		for j := uint(0); j < uint(value.ChildCount()); j++ {
			pair := value.Child(j)
			if pair.Kind() != "pair" {
				continue
			}
			pairKey := pair.ChildByFieldName("key")
			if pairKey != nil {
				keyText := pairKey.Utf8Text(source)
				// Strip quotes from keys like "icon-xs" (string-quoted due to hyphens).
				if isStringLiteral(keyText) {
					keyText = unquoteString(keyText)
				}
				allowedValues = append(allowedValues, keyText)
			}
		}

		variants[variantName] = allowedValues
	}
}

// parseDefaultVariantsObject parses the "defaultVariants" object in a cva() config.
func parseDefaultVariantsObject(defaultsObj *ts.Node, source []byte, defaults map[string]string) {
	if defaultsObj.Kind() != "object" {
		return
	}

	for i := uint(0); i < uint(defaultsObj.ChildCount()); i++ {
		child := defaultsObj.Child(i)
		if child.Kind() != "pair" {
			continue
		}

		key := child.ChildByFieldName("key")
		value := child.ChildByFieldName("value")
		if key == nil || value == nil {
			continue
		}

		keyText := key.Utf8Text(source)
		valueText := value.Utf8Text(source)
		if isStringLiteral(valueText) {
			valueText = unquoteString(valueText)
		}

		defaults[keyText] = valueText
	}
}

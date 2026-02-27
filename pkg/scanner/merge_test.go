package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeProps_EmptyNodeProps(t *testing.T) {
	base := []ExtractedProp{
		{Name: "variant", Type: "string", Required: false, Default: "default"},
	}
	result := mergeProps(base, nil)
	assert.Len(t, result, 1)
	assert.Equal(t, "variant", result[0].Name)
}

func TestMergeProps_FillEmptyDescription(t *testing.T) {
	base := []ExtractedProp{
		{Name: "variant", Type: "string"},
	}
	node := []DocgenProp{
		{Name: "variant", Type: "string", Description: "The visual variant"},
	}
	result := mergeProps(base, node)
	assert.Len(t, result, 1)
	assert.Equal(t, "The visual variant", result[0].Description)
}

func TestMergeProps_TreeSitterDescriptionPreserved(t *testing.T) {
	base := []ExtractedProp{
		{Name: "variant", Type: "string", Description: "Tree-sitter desc"},
	}
	node := []DocgenProp{
		{Name: "variant", Type: "string", Description: "Node desc"},
	}
	result := mergeProps(base, node)
	assert.Equal(t, "Tree-sitter desc", result[0].Description)
}

func TestMergeProps_TreeSitterDefaultWins(t *testing.T) {
	base := []ExtractedProp{
		{Name: "variant", Type: "string", Default: "destructive"},
	}
	node := []DocgenProp{
		{Name: "variant", Type: "string", DefaultValue: "default"},
	}
	result := mergeProps(base, node)
	assert.Equal(t, "destructive", result[0].Default)
}

func TestMergeProps_NodeDefaultFillsEmpty(t *testing.T) {
	base := []ExtractedProp{
		{Name: "variant", Type: "string"},
	}
	node := []DocgenProp{
		{Name: "variant", Type: "string", DefaultValue: "default"},
	}
	result := mergeProps(base, node)
	assert.Equal(t, "default", result[0].Default)
}

func TestMergeProps_TreeSitterAllowedValuesWin(t *testing.T) {
	base := []ExtractedProp{
		{Name: "variant", Type: "string", AllowedValues: []string{"a", "b"}},
	}
	node := []DocgenProp{
		{Name: "variant", Type: "string", AllowedValues: []string{"x", "y", "z"}},
	}
	result := mergeProps(base, node)
	assert.Equal(t, []string{"a", "b"}, result[0].AllowedValues)
}

func TestMergeProps_NodeAllowedValuesFillEmpty(t *testing.T) {
	base := []ExtractedProp{
		{Name: "variant", Type: "string"},
	}
	node := []DocgenProp{
		{Name: "variant", Type: "string", AllowedValues: []string{"x", "y"}},
	}
	result := mergeProps(base, node)
	assert.Equal(t, []string{"x", "y"}, result[0].AllowedValues)
}

func TestMergeProps_NodeRequiredUpgrade(t *testing.T) {
	base := []ExtractedProp{
		{Name: "onClick", Type: "function", Required: false},
	}
	node := []DocgenProp{
		{Name: "onClick", Type: "() => void", Required: true},
	}
	result := mergeProps(base, node)
	assert.True(t, result[0].Required)
}

func TestMergeProps_NodeRequiredDoesNotDowngrade(t *testing.T) {
	// If tree-sitter says required, keep it even if Node says optional.
	base := []ExtractedProp{
		{Name: "children", Type: "ReactNode", Required: true},
	}
	node := []DocgenProp{
		{Name: "children", Type: "ReactNode", Required: false},
	}
	result := mergeProps(base, node)
	assert.True(t, result[0].Required)
}

func TestMergeProps_DeprecatedFromEitherSource(t *testing.T) {
	base := []ExtractedProp{
		{Name: "color", Type: "string"},
	}
	node := []DocgenProp{
		{Name: "color", Type: "string", Deprecated: true},
	}
	result := mergeProps(base, node)
	assert.True(t, result[0].Deprecated)
}

func TestMergeProps_NewPropsFromNode(t *testing.T) {
	base := []ExtractedProp{
		{Name: "variant", Type: "string"},
	}
	node := []DocgenProp{
		{Name: "variant", Type: "string"},
		{Name: "className", Type: "string", Description: "CSS class name"},
		{Name: "disabled", Type: "boolean", Required: false},
	}
	result := mergeProps(base, node)
	assert.Len(t, result, 3)

	// Original prop preserved.
	assert.Equal(t, "variant", result[0].Name)

	// New props from Node.
	assert.Equal(t, "className", result[1].Name)
	assert.Equal(t, "CSS class name", result[1].Description)
	assert.Equal(t, "disabled", result[2].Name)
	assert.Equal(t, "boolean", result[2].Type)
}

func TestMergeProps_TypeFillFromNode(t *testing.T) {
	base := []ExtractedProp{
		{Name: "onChange", Type: ""},
	}
	node := []DocgenProp{
		{Name: "onChange", Type: "() => void"},
	}
	result := mergeProps(base, node)
	assert.Equal(t, "function", result[0].Type)
}

func TestMergeEnrichedProps_ComponentDescription(t *testing.T) {
	components := []DetectedComponent{
		{Name: "Button", FilePath: "/src/button.tsx"},
	}
	propsMap := map[string]*PropExtractionResult{
		"Button": {
			ComponentName: "Button",
			FilePath:      "/src/button.tsx",
			Props:         []ExtractedProp{{Name: "variant", Type: "string"}},
		},
	}
	enriched := &EnrichResult{
		Components: map[string]*DocgenResult{
			"Button": {
				DisplayName: "Button",
				Description: "A clickable button component",
				Props: []DocgenProp{
					{Name: "variant", Type: "string", Description: "Visual style"},
				},
			},
		},
	}

	MergeEnrichedProps(propsMap, enriched, components)

	assert.Equal(t, "A clickable button component", propsMap["Button"].Description)
	assert.Equal(t, "Visual style", propsMap["Button"].Props[0].Description)
}

func TestMergeEnrichedProps_NewComponentFromNode(t *testing.T) {
	components := []DetectedComponent{
		{Name: "Button", FilePath: "/src/button.tsx"},
		{Name: "IconButton", FilePath: "/src/button.tsx"},
	}
	propsMap := map[string]*PropExtractionResult{
		"Button": {
			ComponentName: "Button",
			FilePath:      "/src/button.tsx",
			Props:         []ExtractedProp{{Name: "variant", Type: "string"}},
		},
		// IconButton has no tree-sitter props.
	}
	enriched := &EnrichResult{
		Components: map[string]*DocgenResult{
			"IconButton": {
				DisplayName: "IconButton",
				Description: "Button with icon",
				Props: []DocgenProp{
					{Name: "icon", Type: "ReactNode", Required: true},
				},
			},
		},
	}

	MergeEnrichedProps(propsMap, enriched, components)

	assert.Contains(t, propsMap, "IconButton")
	assert.Equal(t, "Button with icon", propsMap["IconButton"].Description)
	assert.Len(t, propsMap["IconButton"].Props, 1)
	assert.Equal(t, "icon", propsMap["IconButton"].Props[0].Name)
}

func TestMergeEnrichedProps_NilEnriched(t *testing.T) {
	propsMap := map[string]*PropExtractionResult{
		"Button": {ComponentName: "Button", Props: []ExtractedProp{{Name: "x"}}},
	}
	MergeEnrichedProps(propsMap, nil, nil)
	assert.Len(t, propsMap["Button"].Props, 1)
}

func TestSimplifyDocgenType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"string", "string"},
		{"boolean", "boolean"},
		{"() => void", "function"},
		{"(...args: any[]) => any", "function"},
		{"React.ReactNode", "ReactNode"},
		{"ReactElement", "ReactElement"},
		{"enum", "string"},
		{"SomeCustomType", "SomeCustomType"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, simplifyDocgenType(tt.input))
		})
	}
}

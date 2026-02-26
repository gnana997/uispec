package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractCVA_Basic(t *testing.T) {
	propsMap := extractPropsForFixture(t, "cva-component.tsx")

	result, ok := propsMap["Badge"]
	require.True(t, ok, "should extract props for Badge")

	// Build lookup by name.
	byName := make(map[string]ExtractedProp)
	for _, p := range result.Props {
		byName[p.Name] = p
	}

	// "label" comes from the interface directly.
	label, ok := byName["label"]
	require.True(t, ok, "should have label prop")
	assert.Equal(t, "string", label.Type)
	assert.True(t, label.Required)

	// "variant" should have CVA-sourced allowed values.
	variant, ok := byName["variant"]
	require.True(t, ok, "should have variant prop from CVA")
	assert.ElementsMatch(t, []string{"default", "secondary", "destructive", "outline"}, variant.AllowedValues)
	assert.Equal(t, "default", variant.Default)

	// "size" should have CVA-sourced allowed values.
	size, ok := byName["size"]
	require.True(t, ok, "should have size prop from CVA")
	assert.ElementsMatch(t, []string{"sm", "md", "lg"}, size.AllowedValues)
	assert.Equal(t, "md", size.Default)
}

func TestExtractCVA_NoCVA(t *testing.T) {
	propsMap := extractPropsForFixture(t, "button.tsx")

	result := propsMap["Button"]
	require.NotNil(t, result)

	// button.tsx has no cva() calls, so union values should come from the interface only.
	byName := make(map[string]ExtractedProp)
	for _, p := range result.Props {
		byName[p.Name] = p
	}

	// Values should still be present (from interface union types), not from CVA.
	assert.ElementsMatch(t, []string{"default", "destructive", "outline"}, byName["variant"].AllowedValues)
}

func TestExtractCVA_MergeWithInterface(t *testing.T) {
	propsMap := extractPropsForFixture(t, "cva-component.tsx")

	result := propsMap["Badge"]
	require.NotNil(t, result)

	// Check that interface-only prop "label" is present alongside CVA props.
	names := make([]string, 0, len(result.Props))
	for _, p := range result.Props {
		names = append(names, p.Name)
	}
	assert.Contains(t, names, "label", "interface-only prop should be present")
	assert.Contains(t, names, "variant", "CVA prop should be present")
	assert.Contains(t, names, "size", "CVA prop should be present")
}

package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/parser"
)

// extractPropsForFixture is a test helper that runs extraction + detection + prop extraction
// for a single test fixture file.
func extractPropsForFixture(t *testing.T, filename string) map[string]*PropExtractionResult {
	t.Helper()

	ext, cleanup := setupExtractor(t)
	defer cleanup()

	filePath := absTestdata(t, filename)
	files := []string{filePath}
	results, _ := ExtractAll(files, ext, nil)
	require.NotEmpty(t, results, "extraction should return results")

	pm := parser.NewParserManager(nil)
	defer pm.Close()

	components, _ := DetectComponents(results, pm)

	// Create a second ParserManager for prop extraction (detection closes its trees).
	pm2 := parser.NewParserManager(nil)
	defer pm2.Close()

	resultsByFile := make(map[string]*FileExtractionResult)
	for i := range results {
		resultsByFile[results[i].FilePath] = &results[i]
	}

	return ExtractAllProps(components, resultsByFile, pm2)
}

func TestExtractProps_BasicInterface(t *testing.T) {
	propsMap := extractPropsForFixture(t, "button.tsx")

	result, ok := propsMap["Button"]
	require.True(t, ok, "should extract props for Button")
	require.Len(t, result.Props, 3, "Button should have 3 props")

	// Build a lookup by name.
	byName := make(map[string]ExtractedProp)
	for _, p := range result.Props {
		byName[p.Name] = p
	}

	// variant
	variant, ok := byName["variant"]
	require.True(t, ok, "should have variant prop")
	assert.Equal(t, "string", variant.Type)
	assert.False(t, variant.Required)
	assert.ElementsMatch(t, []string{"default", "destructive", "outline"}, variant.AllowedValues)

	// size
	size, ok := byName["size"]
	require.True(t, ok, "should have size prop")
	assert.Equal(t, "string", size.Type)
	assert.False(t, size.Required)
	assert.ElementsMatch(t, []string{"sm", "md", "lg"}, size.AllowedValues)

	// children
	children, ok := byName["children"]
	require.True(t, ok, "should have children prop")
	assert.True(t, children.Required)
}

func TestExtractProps_Defaults(t *testing.T) {
	propsMap := extractPropsForFixture(t, "button.tsx")

	result := propsMap["Button"]
	require.NotNil(t, result)

	byName := make(map[string]ExtractedProp)
	for _, p := range result.Props {
		byName[p.Name] = p
	}

	assert.Equal(t, "default", byName["variant"].Default)
	assert.Equal(t, "md", byName["size"].Default)
	assert.Empty(t, byName["children"].Default)
}

func TestExtractProps_ForwardRef(t *testing.T) {
	propsMap := extractPropsForFixture(t, "forwarded.tsx")

	result, ok := propsMap["Input"]
	require.True(t, ok, "should extract props for Input")
	require.GreaterOrEqual(t, len(result.Props), 3, "Input should have at least 3 props")

	byName := make(map[string]ExtractedProp)
	for _, p := range result.Props {
		byName[p.Name] = p
	}

	assert.Contains(t, byName, "placeholder")
	assert.Contains(t, byName, "value")
	assert.Contains(t, byName, "onChange")

	assert.False(t, byName["placeholder"].Required)
	assert.False(t, byName["value"].Required)
	assert.Equal(t, "function", byName["onChange"].Type)
}

func TestExtractProps_TypeAlias(t *testing.T) {
	propsMap := extractPropsForFixture(t, "type-alias-props.tsx")

	result, ok := propsMap["Tag"]
	require.True(t, ok, "should extract props for Tag")
	require.Len(t, result.Props, 4, "Tag should have 4 props")

	byName := make(map[string]ExtractedProp)
	for _, p := range result.Props {
		byName[p.Name] = p
	}

	name := byName["name"]
	assert.Equal(t, "string", name.Type)
	assert.True(t, name.Required)

	count := byName["count"]
	assert.Equal(t, "number", count.Type)
	assert.False(t, count.Required)

	color := byName["color"]
	assert.Equal(t, "string", color.Type)
	assert.False(t, color.Required)
	assert.Equal(t, "blue", color.Default)

	onRemove := byName["onRemove"]
	assert.Equal(t, "function", onRemove.Type)
	assert.False(t, onRemove.Required)
}

func TestExtractProps_JSDoc(t *testing.T) {
	propsMap := extractPropsForFixture(t, "jsdoc-props.tsx")

	result, ok := propsMap["Alert"]
	require.True(t, ok, "should extract props for Alert")

	byName := make(map[string]ExtractedProp)
	for _, p := range result.Props {
		byName[p.Name] = p
	}

	// Check descriptions.
	assert.Contains(t, byName["message"].Description, "main message")
	assert.Contains(t, byName["severity"].Description, "severity level")
	assert.Contains(t, byName["dismissible"].Description, "dismissed")

	// Check deprecated.
	text, ok := byName["text"]
	require.True(t, ok, "should have text prop")
	assert.True(t, text.Deprecated, "text should be deprecated")
}

func TestExtractProps_NoPropsRef(t *testing.T) {
	propsMap := extractPropsForFixture(t, "memo.tsx")

	result, ok := propsMap["MemoBadge"]
	require.True(t, ok, "should have entry for MemoBadge")
	assert.Empty(t, result.Props, "memo wrapper without inline props should have empty props")
}

func TestExtractProps_CVAScoping(t *testing.T) {
	propsMap := extractPropsForFixture(t, "multi-cva.tsx")

	// MenuButton should have CVA props (variant, size) because it references
	// VariantProps<typeof menuButtonVariants>.
	mbResult, ok := propsMap["MenuButton"]
	require.True(t, ok, "should extract props for MenuButton")
	mbByName := make(map[string]ExtractedProp)
	for _, p := range mbResult.Props {
		mbByName[p.Name] = p
	}
	assert.Contains(t, mbByName, "variant", "MenuButton should have variant prop")
	assert.Contains(t, mbByName, "size", "MenuButton should have size prop")
	assert.ElementsMatch(t, []string{"default", "outline"}, mbByName["variant"].AllowedValues)
	assert.ElementsMatch(t, []string{"default", "sm", "lg"}, mbByName["size"].AllowedValues)

	// Menu should NOT have CVA props — it doesn't reference menuButtonVariants.
	menuResult, ok := propsMap["Menu"]
	require.True(t, ok, "should extract props for Menu")
	for _, p := range menuResult.Props {
		assert.NotEqual(t, "variant", p.Name, "Menu should not have variant prop")
		assert.NotEqual(t, "size", p.Name, "Menu should not have size prop")
	}

	// MenuItem should NOT have CVA props.
	miResult, ok := propsMap["MenuItem"]
	require.True(t, ok, "should extract props for MenuItem")
	for _, p := range miResult.Props {
		assert.NotEqual(t, "variant", p.Name, "MenuItem should not have variant prop")
		assert.NotEqual(t, "size", p.Name, "MenuItem should not have size prop")
	}

	// MenuSeparator should NOT have CVA props.
	msResult, ok := propsMap["MenuSeparator"]
	require.True(t, ok, "should extract props for MenuSeparator")
	for _, p := range msResult.Props {
		assert.NotEqual(t, "variant", p.Name, "MenuSeparator should not have variant prop")
		assert.NotEqual(t, "size", p.Name, "MenuSeparator should not have size prop")
	}
}

func TestExtractProps_CompoundComponent(t *testing.T) {
	propsMap := extractPropsForFixture(t, "dialog.tsx")

	// Should have props for all three components.
	require.Contains(t, propsMap, "Dialog")
	require.Contains(t, propsMap, "DialogTrigger")
	require.Contains(t, propsMap, "DialogContent")

	// Dialog should have 2 props: open, children
	assert.Len(t, propsMap["Dialog"].Props, 2)

	// DialogTrigger should have 1 prop: children
	assert.Len(t, propsMap["DialogTrigger"].Props, 1)

	// DialogContent should have 1 prop: children
	assert.Len(t, propsMap["DialogContent"].Props, 1)
}

func TestExtractProps_ComponentProps(t *testing.T) {
	propsMap := extractPropsForFixture(t, "component-props.tsx")

	// Input: destructured className and type from ComponentProps<"input">
	input, ok := propsMap["Input"]
	require.True(t, ok, "should extract props for Input")
	require.GreaterOrEqual(t, len(input.Props), 2, "Input should have at least 2 destructured props")

	byName := make(map[string]ExtractedProp)
	for _, p := range input.Props {
		byName[p.Name] = p
	}
	assert.Contains(t, byName, "className")
	assert.Contains(t, byName, "type")

	// Checkbox: destructured className from ComponentPropsWithoutRef<"input">
	cb, ok := propsMap["Checkbox"]
	require.True(t, ok, "should extract props for Checkbox")
	require.GreaterOrEqual(t, len(cb.Props), 1, "Checkbox should have at least 1 destructured prop")

	cbByName := make(map[string]ExtractedProp)
	for _, p := range cb.Props {
		cbByName[p.Name] = p
	}
	assert.Contains(t, cbByName, "className")
}

func TestExtractProps_ComponentPropsIntersection(t *testing.T) {
	propsMap := extractPropsForFixture(t, "component-props.tsx")

	// SelectTrigger: has inline intersection { size?: "sm" | "default" }
	st, ok := propsMap["SelectTrigger"]
	require.True(t, ok, "should extract props for SelectTrigger")

	byName := make(map[string]ExtractedProp)
	for _, p := range st.Props {
		byName[p.Name] = p
	}

	// "size" comes from the inline object_type with full type info.
	size, ok := byName["size"]
	require.True(t, ok, "should have size prop")
	assert.False(t, size.Required)
	assert.ElementsMatch(t, []string{"sm", "default"}, size.AllowedValues)
	assert.Equal(t, "default", size.Default)

	// "className" and "children" come from destructuring.
	assert.Contains(t, byName, "className")
	assert.Contains(t, byName, "children")
}

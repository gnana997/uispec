package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/parser"
)

func extractAndDetect(t *testing.T, fixture string) ([]DetectedComponent, []CompoundGroup) {
	t.Helper()
	ext, cleanup := setupExtractor(t)
	defer cleanup()

	pm := parser.NewParserManager(nil)
	defer pm.Close()

	files := []string{absTestdata(t, fixture)}
	results, failed := ExtractAll(files, ext, nil)
	require.Equal(t, 0, failed)
	require.Len(t, results, 1)

	return DetectComponents(results, pm)
}

func TestDetectComponents_FunctionComponent(t *testing.T) {
	comps, _ := extractAndDetect(t, "button.tsx")

	require.Len(t, comps, 1)
	assert.Equal(t, "Button", comps[0].Name)
	assert.Equal(t, ComponentKindFunction, comps[0].Kind)
	assert.True(t, comps[0].IsExported)

	// Should detect props ref.
	require.NotNil(t, comps[0].PropsRef)
	assert.Equal(t, "ButtonProps", comps[0].PropsRef.TypeName)
	assert.NotNil(t, comps[0].PropsRef.Symbol, "should find matching interface")
}

func TestDetectComponents_ArrowComponent(t *testing.T) {
	comps, _ := extractAndDetect(t, "arrow.tsx")

	require.Len(t, comps, 1)
	assert.Equal(t, "Card", comps[0].Name)
	assert.Equal(t, ComponentKindFunction, comps[0].Kind)
	assert.True(t, comps[0].IsExported)

	require.NotNil(t, comps[0].PropsRef)
	assert.Equal(t, "CardProps", comps[0].PropsRef.TypeName)
}

func TestDetectComponents_ForwardRef(t *testing.T) {
	comps, _ := extractAndDetect(t, "forwarded.tsx")

	require.Len(t, comps, 1)
	assert.Equal(t, "Input", comps[0].Name)
	assert.Equal(t, ComponentKindForwardRef, comps[0].Kind)
	assert.True(t, comps[0].IsExported)

	require.NotNil(t, comps[0].PropsRef)
	assert.Equal(t, "InputProps", comps[0].PropsRef.TypeName)
}

func TestDetectComponents_Memo(t *testing.T) {
	comps, _ := extractAndDetect(t, "memo.tsx")

	require.Len(t, comps, 1)
	assert.Equal(t, "MemoBadge", comps[0].Name)
	assert.Equal(t, ComponentKindMemo, comps[0].Kind)
	assert.True(t, comps[0].IsExported)
}

func TestDetectComponents_ClassComponent(t *testing.T) {
	comps, _ := extractAndDetect(t, "class-component.tsx")

	require.Len(t, comps, 1)
	assert.Equal(t, "Counter", comps[0].Name)
	assert.Equal(t, ComponentKindClass, comps[0].Kind)
	assert.True(t, comps[0].IsExported)

	require.NotNil(t, comps[0].PropsRef)
	assert.Equal(t, "CounterProps", comps[0].PropsRef.TypeName)
}

func TestDetectComponents_IgnoresUtilities(t *testing.T) {
	comps, _ := extractAndDetect(t, "utility.ts")
	assert.Empty(t, comps, "utility functions should not be detected as components")
}

func TestDetectComponents_IgnoresUnexported(t *testing.T) {
	comps, _ := extractAndDetect(t, "unexported.tsx")
	assert.Empty(t, comps, "unexported components should not be detected")
}

func TestDetectComponents_CompoundGroup(t *testing.T) {
	comps, groups := extractAndDetect(t, "dialog.tsx")

	assert.Len(t, comps, 3, "should detect Dialog, DialogTrigger, DialogContent")

	// Verify all three components are detected.
	names := make([]string, len(comps))
	for i, c := range comps {
		names[i] = c.Name
	}
	assert.Contains(t, names, "Dialog")
	assert.Contains(t, names, "DialogTrigger")
	assert.Contains(t, names, "DialogContent")

	// Should form one compound group.
	require.Len(t, groups, 1)
	assert.Equal(t, "Dialog", groups[0].Parent.Name)
	assert.Len(t, groups[0].SubComponents, 2)
}

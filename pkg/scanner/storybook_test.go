package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gnana997/uispec/pkg/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverStoryFiles(t *testing.T) {
	testdataDir := absTestdata(t, ".")

	files, err := DiscoverStoryFiles(testdataDir, nil)
	require.NoError(t, err)

	// Should find the Button.stories.tsx fixture.
	require.Len(t, files, 1)
	assert.Contains(t, files[0], "Button.stories.tsx")
}

func TestDiscoverStoryFiles_WithExcludes(t *testing.T) {
	testdataDir := absTestdata(t, ".")

	files, err := DiscoverStoryFiles(testdataDir, []string{"**/*.stories.*"})
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestDiscoverStoryFiles_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	files, err := DiscoverStoryFiles(tmpDir, nil)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestIsStoryFile(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"Button.stories.tsx", true},
		{"Button.stories.ts", true},
		{"Button.stories.js", true},
		{"Button.stories.jsx", true},
		{"Button.stories.css", false},
		{"Button.test.tsx", false},
		{"button.tsx", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isStoryFile(tt.name))
		})
	}
}

func TestRunStorybookExtraction_Integration(t *testing.T) {
	runtime, found := findNodeRuntime()
	if !found {
		t.Skip("node runtime not found")
	}

	// Verify the embedded worker script exists.
	if len(storybookScript) == 0 {
		t.Fatal("storybookScript embed is empty — run 'make docgen-bundle' first")
	}

	testdataDir := absTestdata(t, ".")
	storyFile := filepath.Join(testdataDir, "Button.stories.tsx")
	require.FileExists(t, storyFile)

	result, err := RunStorybookExtraction(testdataDir, []string{storyFile}, runtime, nil)
	require.NoError(t, err)
	require.Len(t, result.Results, 1)

	sf := result.Results[0]
	assert.Equal(t, "Button", sf.ComponentName)
	assert.Equal(t, "./button", sf.ComponentImport)
	assert.Equal(t, "Components/Button", sf.Title)
	require.Len(t, sf.Stories, 3)

	// Primary: args-only with children
	primary := sf.Stories[0]
	assert.Equal(t, "Primary", primary.ExportName)
	assert.Contains(t, primary.Code, `variant="default"`)
	assert.Contains(t, primary.Code, `size="lg"`)
	assert.Contains(t, primary.Code, "Click me")
	assert.False(t, primary.HasRenderFunction)

	// Secondary: args-only, no children
	secondary := sf.Stories[1]
	assert.Equal(t, "Secondary", secondary.ExportName)
	assert.Contains(t, secondary.Code, `variant="destructive"`)
	assert.Contains(t, secondary.Code, "/>")
	assert.False(t, secondary.HasRenderFunction)

	// WithRender: has render function
	withRender := sf.Stories[2]
	assert.Equal(t, "WithRender", withRender.ExportName)
	assert.True(t, withRender.HasRenderFunction)
	assert.Contains(t, withRender.Code, "<Button")
}

func TestBuildExamplesMap(t *testing.T) {
	components := []DetectedComponent{
		{Name: "Button", FilePath: "/test/button.tsx"},
		{Name: "Card", FilePath: "/test/card.tsx"},
	}

	sbResult := &StorybookExtractionResult{
		Results: []storyFileResult{
			{
				ComponentName: "Button",
				Stories: []storyInfo{
					{Name: "Primary", ExportName: "Primary", Code: `<Button variant="primary" />`},
					{Name: "Secondary", ExportName: "Secondary", Code: `<Button variant="secondary" />`},
				},
			},
			{
				// UnknownComponent — should be skipped
				ComponentName: "Unknown",
				Stories: []storyInfo{
					{Name: "Default", ExportName: "Default", Code: `<Unknown />`},
				},
			},
		},
	}

	examples := BuildExamplesMap(sbResult, components)

	// Button should have 2 examples.
	require.Len(t, examples["Button"], 2)
	assert.Equal(t, "Primary", examples["Button"][0].Title)
	assert.Equal(t, `<Button variant="primary" />`, examples["Button"][0].Code)

	// Card has no stories.
	assert.Empty(t, examples["Card"])

	// Unknown was skipped.
	assert.Empty(t, examples["Unknown"])
}

func TestBuildExamplesMap_NilResult(t *testing.T) {
	examples := BuildExamplesMap(nil, nil)
	assert.Nil(t, examples)
}

func TestBuildExamplesMap_PlayFunction(t *testing.T) {
	components := []DetectedComponent{
		{Name: "Button", FilePath: "/test/button.tsx"},
	}

	sbResult := &StorybookExtractionResult{
		Results: []storyFileResult{
			{
				ComponentName: "Button",
				Stories: []storyInfo{
					{Name: "Interactive", ExportName: "Interactive", Code: `<Button />`, HasPlayFunction: true},
				},
			},
		},
	}

	examples := BuildExamplesMap(sbResult, components)
	require.Len(t, examples["Button"], 1)
	assert.Equal(t, "Interactive story with play function", examples["Button"][0].Description)
}

func TestRunStorybookExtraction_NoFiles(t *testing.T) {
	result, err := RunStorybookExtraction("/tmp", nil, "node", nil)
	require.NoError(t, err)
	assert.Empty(t, result.Results)
}

func TestRunStorybookExtraction_InvalidFile(t *testing.T) {
	runtime, found := findNodeRuntime()
	if !found {
		t.Skip("node runtime not found")
	}

	if len(storybookScript) == 0 {
		t.Skip("storybookScript embed is empty — run 'make docgen-bundle' first")
	}

	// Create a file that's not valid CSF.
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.stories.tsx")
	require.NoError(t, os.WriteFile(badFile, []byte("not valid code {{{"), 0644))

	result, err := RunStorybookExtraction(tmpDir, []string{badFile}, runtime, nil)
	require.NoError(t, err)
	// Should return empty results (bad file skipped, not error).
	assert.Empty(t, result.Results)
}

func TestBuildCatalog_WithExamples(t *testing.T) {
	testdataDir := absTestdata(t, ".")
	cfg := CatalogBuildConfig{
		Name:         "test-lib",
		ImportPrefix: "@/components/ui",
		RootDir:      testdataDir,
	}

	scanResult, propsMap := buildCatalogForFixtures(t, []string{"button.tsx"}, cfg)

	examples := map[string][]catalog.Example{
		"Button": {
			{Title: "Primary", Code: `<Button variant="default">Click me</Button>`},
			{Title: "Outline", Code: `<Button variant="outline" />`},
		},
	}

	cat, err := BuildCatalog(scanResult, propsMap, cfg, nil, examples)
	require.NoError(t, err)

	require.Len(t, cat.Components, 1)
	require.Len(t, cat.Components[0].Examples, 2)
	assert.Equal(t, "Primary", cat.Components[0].Examples[0].Title)
	assert.Contains(t, cat.Components[0].Examples[0].Code, "Click me")
}

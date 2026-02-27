package scanner

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/parser"
)

// buildCatalogForFixtures runs the full pipeline (extract → detect → props → catalog)
// for the given fixture files and returns the built catalog.
func buildCatalogForFixtures(t *testing.T, fixtures []string, cfg CatalogBuildConfig) (*ScanResult, map[string]*PropExtractionResult) {
	t.Helper()

	ext, cleanup := setupExtractor(t)
	defer cleanup()

	var files []string
	for _, f := range fixtures {
		files = append(files, absTestdata(t, f))
	}

	results, _ := ExtractAll(files, ext, nil)
	require.NotEmpty(t, results)

	pm := parser.NewParserManager(nil)
	defer pm.Close()

	components, groups := DetectComponents(results, pm)

	pm2 := parser.NewParserManager(nil)
	defer pm2.Close()

	resultsByFile := make(map[string]*FileExtractionResult)
	for i := range results {
		resultsByFile[results[i].FilePath] = &results[i]
	}

	propsMap := ExtractAllProps(components, resultsByFile, pm2)

	scanResult := &ScanResult{
		Components:     components,
		CompoundGroups: groups,
	}

	return scanResult, propsMap
}

func TestBuildCatalog_SingleComponent(t *testing.T) {
	testdataDir := absTestdata(t, ".")
	cfg := CatalogBuildConfig{
		Name:         "test-lib",
		ImportPrefix: "@/components/ui",
		RootDir:      testdataDir,
	}

	scanResult, propsMap := buildCatalogForFixtures(t, []string{"button.tsx"}, cfg)
	cat, err := BuildCatalog(scanResult, propsMap, cfg, nil)
	require.NoError(t, err)

	assert.Equal(t, "test-lib", cat.Name)
	assert.Equal(t, "1.0", cat.Version)
	assert.Equal(t, "react", cat.Framework)

	require.Len(t, cat.Components, 1)
	comp := cat.Components[0]
	assert.Equal(t, "Button", comp.Name)
	assert.Equal(t, "@/components/ui/button", comp.ImportPath)
	assert.Contains(t, comp.ImportedNames, "Button")
	assert.GreaterOrEqual(t, len(comp.Props), 3)
}

func TestBuildCatalog_CompoundComponent(t *testing.T) {
	testdataDir := absTestdata(t, ".")
	cfg := CatalogBuildConfig{
		Name:         "test-lib",
		ImportPrefix: "@/components/ui",
		RootDir:      testdataDir,
	}

	scanResult, propsMap := buildCatalogForFixtures(t, []string{"dialog.tsx"}, cfg)
	cat, err := BuildCatalog(scanResult, propsMap, cfg, nil)
	require.NoError(t, err)

	// Should have 1 top-level component (Dialog).
	require.Len(t, cat.Components, 1)
	dialog := cat.Components[0]
	assert.Equal(t, "Dialog", dialog.Name)

	// Should have 2 sub-components.
	require.Len(t, dialog.SubComponents, 2)

	subNames := make([]string, len(dialog.SubComponents))
	for i, s := range dialog.SubComponents {
		subNames[i] = s.Name
	}
	assert.ElementsMatch(t, []string{"DialogTrigger", "DialogContent"}, subNames)

	// ImportedNames should include all 3.
	assert.ElementsMatch(t, []string{"Dialog", "DialogTrigger", "DialogContent"}, dialog.ImportedNames)

	// Sub-components should reference parent.
	for _, sub := range dialog.SubComponents {
		assert.Equal(t, []string{"Dialog"}, sub.AllowedParents)
	}
}

func TestBuildCatalog_ImportPath(t *testing.T) {
	testdataDir := absTestdata(t, ".")

	tests := []struct {
		name     string
		prefix   string
		expected string
	}{
		{
			name:     "with prefix",
			prefix:   "@/components/ui",
			expected: "@/components/ui/button",
		},
		{
			name:     "no prefix",
			prefix:   "",
			expected: "./button",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := CatalogBuildConfig{
				Name:         "test-lib",
				ImportPrefix: tt.prefix,
				RootDir:      testdataDir,
			}

			scanResult, propsMap := buildCatalogForFixtures(t, []string{"button.tsx"}, cfg)
			cat, err := BuildCatalog(scanResult, propsMap, cfg, nil)
			require.NoError(t, err)
			require.Len(t, cat.Components, 1)
			assert.Equal(t, tt.expected, cat.Components[0].ImportPath)
		})
	}
}

func TestBuildCatalog_AutoCategories(t *testing.T) {
	testdataDir := absTestdata(t, ".")
	cfg := CatalogBuildConfig{
		Name:         "test-lib",
		ImportPrefix: "@/components",
		RootDir:      filepath.Dir(testdataDir), // parent of testdata, so testdata becomes a subdir
	}

	scanResult, propsMap := buildCatalogForFixtures(t, []string{"button.tsx", "dialog.tsx"}, cfg)
	cat, err := BuildCatalog(scanResult, propsMap, cfg, nil)
	require.NoError(t, err)

	// Since both files are in testdata/ subdirectory, they should be in the "testdata" category.
	require.Len(t, cat.Categories, 1)
	assert.Equal(t, "testdata", cat.Categories[0].Name)
}

func TestBuildCatalog_Validates(t *testing.T) {
	testdataDir := absTestdata(t, ".")
	cfg := CatalogBuildConfig{
		Name:         "test-lib",
		ImportPrefix: "@/components/ui",
		RootDir:      testdataDir,
	}

	scanResult, propsMap := buildCatalogForFixtures(t, []string{"button.tsx", "forwarded.tsx"}, cfg)
	cat, err := BuildCatalog(scanResult, propsMap, cfg, nil)
	require.NoError(t, err)

	// The built catalog should pass validation.
	errs := cat.Validate()
	assert.Empty(t, errs, "generated catalog should pass validation")
}

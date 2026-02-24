package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helpers ---

func minimalValidCatalog() *Catalog {
	return &Catalog{
		Name:    "test",
		Version: "1.0",
		Categories: []Category{
			{Name: "actions", Components: []string{"Button"}},
		},
		Components: []Component{
			{
				Name:          "Button",
				Description:   "A button",
				Category:      "actions",
				ImportPath:    "@/components/ui/button",
				ImportedNames: []string{"Button"},
				Props: []Prop{
					{Name: "variant", Type: "string"},
				},
			},
		},
		Guidelines: []Guideline{
			{Rule: "test-rule", Description: "A test rule", Severity: "warning"},
		},
	}
}

func compoundCatalog() *Catalog {
	return &Catalog{
		Name:    "test-compound",
		Version: "1.0",
		Categories: []Category{
			{Name: "overlay", Components: []string{"Dialog"}},
		},
		Components: []Component{
			{
				Name:          "Dialog",
				Description:   "A dialog",
				Category:      "overlay",
				ImportPath:    "@/components/ui/dialog",
				ImportedNames: []string{"Dialog", "DialogTrigger", "DialogContent", "DialogTitle"},
				SubComponents: []SubComponent{
					{
						Name:           "DialogTrigger",
						Description:    "Opens the dialog",
						AllowedParents: []string{"Dialog"},
					},
					{
						Name:           "DialogContent",
						Description:    "Dialog content container",
						MustContain:    []string{"DialogTitle"},
						AllowedParents: []string{"Dialog"},
					},
					{
						Name:           "DialogTitle",
						Description:    "Accessible title",
						AllowedParents: []string{"DialogContent"},
					},
				},
				Guidelines: []Guideline{
					{Rule: "dialog-title", Description: "Must have title", Severity: "error"},
				},
			},
		},
	}
}

func writeTempCatalog(t *testing.T, catalog *Catalog) string {
	t.Helper()
	data, err := json.MarshalIndent(catalog, "", "  ")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "catalog.json")
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

// --- Validate() tests ---

func TestValidate_MinimalValid(t *testing.T) {
	c := minimalValidCatalog()
	errs := c.Validate()
	assert.Empty(t, errs)
}

func TestValidate_CompoundValid(t *testing.T) {
	c := compoundCatalog()
	errs := c.Validate()
	assert.Empty(t, errs)
}

func TestValidate_EmptyName(t *testing.T) {
	c := minimalValidCatalog()
	c.Name = ""
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "catalog name is required")
}

func TestValidate_EmptyVersion(t *testing.T) {
	c := minimalValidCatalog()
	c.Version = ""
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "catalog version is required")
}

func TestValidate_DuplicateComponentName(t *testing.T) {
	c := minimalValidCatalog()
	c.Components = append(c.Components, Component{
		Name:          "Button",
		ImportPath:    "@/components/ui/button2",
		ImportedNames: []string{"Button"},
	})
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "duplicate component name")
}

func TestValidate_DuplicateSubComponentName(t *testing.T) {
	c := &Catalog{
		Name:    "test",
		Version: "1.0",
		Categories: []Category{
			{Name: "overlay", Components: []string{"Dialog", "Sheet"}},
		},
		Components: []Component{
			{
				Name:          "Dialog",
				Category:      "overlay",
				ImportPath:    "@/components/ui/dialog",
				ImportedNames: []string{"Dialog", "SharedSub"},
				SubComponents: []SubComponent{
					{Name: "SharedSub", Description: "First"},
				},
			},
			{
				Name:          "Sheet",
				Category:      "overlay",
				ImportPath:    "@/components/ui/sheet",
				ImportedNames: []string{"Sheet", "SharedSub"},
				SubComponents: []SubComponent{
					{Name: "SharedSub", Description: "Duplicate"},
				},
			},
		},
	}
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "duplicate sub-component name")
}

func TestValidate_SubComponentCollidesWithComponent(t *testing.T) {
	c := &Catalog{
		Name:    "test",
		Version: "1.0",
		Categories: []Category{
			{Name: "all", Components: []string{"Button", "Card"}},
		},
		Components: []Component{
			{
				Name:          "Button",
				Category:      "all",
				ImportPath:    "@/components/ui/button",
				ImportedNames: []string{"Button"},
			},
			{
				Name:          "Card",
				Category:      "all",
				ImportPath:    "@/components/ui/card",
				ImportedNames: []string{"Card", "Button"},
				SubComponents: []SubComponent{
					{Name: "Button", Description: "Collides with top-level Button"},
				},
			},
		},
	}
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "collides with a top-level component")
}

func TestValidate_InvalidCategoryReference(t *testing.T) {
	c := minimalValidCatalog()
	c.Components[0].Category = "nonexistent"
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "unknown category")
}

func TestValidate_CategoryReferencesNonexistentComponent(t *testing.T) {
	c := minimalValidCatalog()
	c.Categories[0].Components = append(c.Categories[0].Components, "NonexistentComponent")
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "non-existent component")
}

func TestValidate_MissingComponentName(t *testing.T) {
	c := minimalValidCatalog()
	c.Components[0].Name = ""
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "name is required")
}

func TestValidate_MissingImportPath(t *testing.T) {
	c := minimalValidCatalog()
	c.Components[0].ImportPath = ""
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "import_path is required")
}

func TestValidate_MissingImportedNames(t *testing.T) {
	c := minimalValidCatalog()
	c.Components[0].ImportedNames = nil
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "imported_names must have at least one entry")
}

func TestValidate_InvalidGuidelineSeverity(t *testing.T) {
	c := minimalValidCatalog()
	c.Guidelines[0].Severity = "critical"
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "invalid severity")
}

func TestValidate_InvalidComponentGuidelineSeverity(t *testing.T) {
	c := compoundCatalog()
	c.Components[0].Guidelines[0].Severity = "fatal"
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "invalid severity")
}

func TestValidate_PropMissingName(t *testing.T) {
	c := minimalValidCatalog()
	c.Components[0].Props[0].Name = ""
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "name is required")
}

func TestValidate_PropMissingType(t *testing.T) {
	c := minimalValidCatalog()
	c.Components[0].Props[0].Type = ""
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "type is required")
}

func TestValidate_DuplicateCategoryName(t *testing.T) {
	c := minimalValidCatalog()
	c.Categories = append(c.Categories, Category{Name: "actions", Components: []string{}})
	errs := c.Validate()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "duplicate category name")
}

// --- BuildIndex() tests ---

func TestBuildIndex_ComponentByName(t *testing.T) {
	c := minimalValidCatalog()
	idx := c.BuildIndex()

	comp, ok := idx.ComponentByName["Button"]
	require.True(t, ok)
	assert.Equal(t, "Button", comp.Name)
}

func TestBuildIndex_SubComponentByName(t *testing.T) {
	c := compoundCatalog()
	idx := c.BuildIndex()

	parent, ok := idx.SubComponentByName["DialogTrigger"]
	require.True(t, ok)
	assert.Equal(t, "Dialog", parent.Name)

	parent, ok = idx.SubComponentByName["DialogContent"]
	require.True(t, ok)
	assert.Equal(t, "Dialog", parent.Name)
}

func TestBuildIndex_SubComponentDef(t *testing.T) {
	c := compoundCatalog()
	idx := c.BuildIndex()

	sub, ok := idx.SubComponentDef["DialogContent"]
	require.True(t, ok)
	assert.Equal(t, "DialogContent", sub.Name)
	assert.Equal(t, []string{"DialogTitle"}, sub.MustContain)
}

func TestBuildIndex_CategoryByName(t *testing.T) {
	c := minimalValidCatalog()
	idx := c.BuildIndex()

	cat, ok := idx.CategoryByName["actions"]
	require.True(t, ok)
	assert.Equal(t, "actions", cat.Name)
}

func TestBuildIndex_ComponentsByCategory(t *testing.T) {
	c := compoundCatalog()
	idx := c.BuildIndex()

	comps, ok := idx.ComponentsByCategory["overlay"]
	require.True(t, ok)
	assert.Len(t, comps, 1)
	assert.Equal(t, "Dialog", comps[0].Name)
}

// --- LoadFromFile() tests ---

func TestLoadFromFile_ValidCatalog(t *testing.T) {
	path := writeTempCatalog(t, minimalValidCatalog())
	cat, idx, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "test", cat.Name)
	assert.NotNil(t, idx)
	assert.Contains(t, idx.ComponentByName, "Button")
}

func TestLoadFromFile_FileNotFound(t *testing.T) {
	_, _, err := LoadFromFile("/nonexistent/path/catalog.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read catalog file")
}

func TestLoadFromFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{invalid json}"), 0644))
	_, _, err := LoadFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse catalog JSON")
}

func TestLoadFromFile_ValidationFailure(t *testing.T) {
	bad := &Catalog{
		Name:    "", // missing required field
		Version: "1.0",
	}
	path := writeTempCatalog(t, bad)
	_, _, err := LoadFromFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog validation failed")
}

// --- Integration test with real shadcn catalog ---

func TestLoadFromFile_ShadcnCatalog(t *testing.T) {
	// Find the catalog relative to this test file.
	catalogPath := filepath.Join("..", "..", "catalogs", "shadcn", "catalog.json")
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		t.Skip("shadcn catalog not found at", catalogPath)
	}

	cat, idx, err := LoadFromFile(catalogPath)
	require.NoError(t, err, "shadcn catalog should load without errors")

	// Verify catalog metadata.
	assert.Equal(t, "shadcn/ui", cat.Name)
	assert.Equal(t, "react", cat.Framework)

	// Verify component count.
	assert.Len(t, cat.Components, 30, "expected 30 components")

	// Verify 7 categories.
	assert.Len(t, cat.Categories, 7, "expected 7 categories")

	// Spot-check Button.
	button, ok := idx.ComponentByName["Button"]
	require.True(t, ok, "Button should be in index")
	assert.Equal(t, "@/components/ui/button", button.ImportPath)
	assert.NotEmpty(t, button.Props, "Button should have props")

	// Check Button has variant prop with allowed values.
	var variantProp *Prop
	for i, p := range button.Props {
		if p.Name == "variant" {
			variantProp = &button.Props[i]
			break
		}
	}
	require.NotNil(t, variantProp, "Button should have variant prop")
	assert.Contains(t, variantProp.AllowedValues, "default")
	assert.Contains(t, variantProp.AllowedValues, "destructive")

	// Spot-check Dialog compound component.
	dialog, ok := idx.ComponentByName["Dialog"]
	require.True(t, ok, "Dialog should be in index")
	assert.NotEmpty(t, dialog.SubComponents, "Dialog should have sub-components")

	// Verify DialogContent is indexed.
	dialogParent, ok := idx.SubComponentByName["DialogContent"]
	require.True(t, ok, "DialogContent should be in sub-component index")
	assert.Equal(t, "Dialog", dialogParent.Name)

	// Verify DialogContent must_contain.
	dialogContentDef, ok := idx.SubComponentDef["DialogContent"]
	require.True(t, ok)
	assert.Contains(t, dialogContentDef.MustContain, "DialogTitle")

	// Verify tokens (expanded to include foreground pairs, chart, sidebar).
	assert.GreaterOrEqual(t, len(cat.Tokens), 30, "should have at least 30 design tokens")

	// Verify global guidelines (expanded with info-level guidelines).
	assert.GreaterOrEqual(t, len(cat.Guidelines), 10, "should have at least 10 global guidelines")

	// Verify examples exist on components.
	assert.NotEmpty(t, button.Examples, "Button should have examples")
	assert.NotEmpty(t, button.Examples[0].Code, "Button example should have code")
	assert.NotEmpty(t, dialog.Examples, "Dialog should have examples")

	// Verify all 30 components have at least one example.
	for _, comp := range cat.Components {
		assert.NotEmpty(t, comp.Examples, "%s should have examples", comp.Name)
	}

	// Count total sub-components indexed.
	assert.Greater(t, len(idx.SubComponentByName), 50, "compound components should produce many sub-component index entries")
}

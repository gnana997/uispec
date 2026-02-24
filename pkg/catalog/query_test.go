package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func testQueryService() *QueryService {
	cat := &Catalog{
		Name:    "test",
		Version: "1.0",
		Categories: []Category{
			{Name: "actions", Components: []string{"Button"}},
			{Name: "overlay", Components: []string{"Dialog"}},
		},
		Components: []Component{
			{
				Name:          "Button",
				Description:   "A clickable button",
				Category:      "actions",
				ImportPath:    "@/components/ui/button",
				ImportedNames: []string{"Button"},
				Props: []Prop{
					{Name: "variant", Type: "string"},
					{Name: "size", Type: "string"},
				},
			},
			{
				Name:          "Dialog",
				Description:   "A modal dialog overlay",
				Category:      "overlay",
				ImportPath:    "@/components/ui/dialog",
				ImportedNames: []string{"Dialog", "DialogTrigger", "DialogContent", "DialogTitle"},
				SubComponents: []SubComponent{
					{Name: "DialogTrigger", Description: "Opens the dialog", AllowedParents: []string{"Dialog"}},
					{Name: "DialogContent", Description: "Dialog content container", AllowedParents: []string{"Dialog"}, MustContain: []string{"DialogTitle"}},
					{Name: "DialogTitle", Description: "Accessible title", AllowedParents: []string{"DialogContent"}},
				},
				Guidelines: []Guideline{
					{Rule: "dialog-title", Description: "Must have title", Severity: "error"},
				},
			},
		},
		Tokens: []Token{
			{Name: "background", Value: "hsl(var(--background))", Category: "color"},
			{Name: "foreground", Value: "hsl(var(--foreground))", Category: "color"},
			{Name: "chart-1", Value: "hsl(var(--chart-1))", Category: "chart"},
			{Name: "radius", Value: "var(--radius)", Category: "border"},
		},
		Guidelines: []Guideline{
			{Rule: "use-design-tokens", Description: "Use design tokens", Severity: "warning"},
			{Rule: "import-from-ui", Description: "Import from @/components/ui", Severity: "error"},
		},
	}

	idx := cat.BuildIndex()
	return NewQueryService(cat, idx)
}

// --- ListCategories ---

func TestListCategories(t *testing.T) {
	qs := testQueryService()
	cats := qs.ListCategories()
	assert.Len(t, cats, 2)
	assert.Equal(t, "actions", cats[0].Name)
	assert.Equal(t, "overlay", cats[1].Name)
}

// --- ListComponents ---

func TestListComponents_NoFilter(t *testing.T) {
	qs := testQueryService()
	comps := qs.ListComponents("", "")
	assert.Len(t, comps, 2)
}

func TestListComponents_ByCategory(t *testing.T) {
	qs := testQueryService()
	comps := qs.ListComponents("actions", "")
	require.Len(t, comps, 1)
	assert.Equal(t, "Button", comps[0].Name)
}

func TestListComponents_ByKeyword(t *testing.T) {
	qs := testQueryService()
	comps := qs.ListComponents("", "modal")
	require.Len(t, comps, 1)
	assert.Equal(t, "Dialog", comps[0].Name)
}

func TestListComponents_ByCategoryAndKeyword(t *testing.T) {
	qs := testQueryService()
	// "overlay" category + keyword "modal" -> Dialog
	comps := qs.ListComponents("overlay", "modal")
	require.Len(t, comps, 1)
	assert.Equal(t, "Dialog", comps[0].Name)

	// "actions" category + keyword "modal" -> no match
	comps = qs.ListComponents("actions", "modal")
	assert.Empty(t, comps)
}

func TestListComponents_KeywordCaseInsensitive(t *testing.T) {
	qs := testQueryService()
	comps := qs.ListComponents("", "BUTTON")
	require.Len(t, comps, 1)
	assert.Equal(t, "Button", comps[0].Name)
}

func TestListComponents_NoMatch(t *testing.T) {
	qs := testQueryService()
	comps := qs.ListComponents("", "nonexistent")
	assert.Empty(t, comps)
	assert.NotNil(t, comps) // empty slice, not nil
}

// --- GetComponent ---

func TestGetComponent_Found(t *testing.T) {
	qs := testQueryService()
	comp, ok := qs.GetComponent("Button")
	require.True(t, ok)
	assert.Equal(t, "Button", comp.Name)
}

func TestGetComponent_SubComponent(t *testing.T) {
	qs := testQueryService()
	comp, ok := qs.GetComponent("DialogContent")
	require.True(t, ok)
	assert.Equal(t, "Dialog", comp.Name) // returns parent
}

func TestGetComponent_NotFound(t *testing.T) {
	qs := testQueryService()
	_, ok := qs.GetComponent("NonExistent")
	assert.False(t, ok)
}

// --- GetComponentsByNames ---

func TestGetComponentsByNames(t *testing.T) {
	qs := testQueryService()
	comps := qs.GetComponentsByNames([]string{"Button", "Dialog"})
	assert.Len(t, comps, 2)
}

func TestGetComponentsByNames_Dedup(t *testing.T) {
	qs := testQueryService()
	comps := qs.GetComponentsByNames([]string{"Button", "Button", "Dialog"})
	assert.Len(t, comps, 2)
}

func TestGetComponentsByNames_SkipsUnknown(t *testing.T) {
	qs := testQueryService()
	comps := qs.GetComponentsByNames([]string{"Button", "NonExistent"})
	require.Len(t, comps, 1)
	assert.Equal(t, "Button", comps[0].Name)
}

// --- GetTokens ---

func TestGetTokens_All(t *testing.T) {
	qs := testQueryService()
	tokens := qs.GetTokens("")
	assert.Len(t, tokens, 4)
}

func TestGetTokens_ByCategory(t *testing.T) {
	qs := testQueryService()
	tokens := qs.GetTokens("color")
	require.Len(t, tokens, 2)
	assert.Equal(t, "background", tokens[0].Name)
	assert.Equal(t, "foreground", tokens[1].Name)
}

func TestGetTokens_NoMatch(t *testing.T) {
	qs := testQueryService()
	tokens := qs.GetTokens("nonexistent")
	assert.Empty(t, tokens)
}

// --- GetGuidelines ---

func TestGetGuidelines_Global(t *testing.T) {
	qs := testQueryService()
	guidelines := qs.GetGuidelines("")
	assert.Len(t, guidelines, 2)
}

func TestGetGuidelines_ForComponent(t *testing.T) {
	qs := testQueryService()
	guidelines := qs.GetGuidelines("Dialog")
	// 2 global + 1 component-level
	assert.Len(t, guidelines, 3)
}

func TestGetGuidelines_UnknownComponent(t *testing.T) {
	qs := testQueryService()
	guidelines := qs.GetGuidelines("NonExistent")
	// Falls back to global only
	assert.Len(t, guidelines, 2)
}

// --- SearchComponents ---

func TestSearchComponents_ByName(t *testing.T) {
	qs := testQueryService()
	results := qs.SearchComponents("button")
	require.Len(t, results, 1)
	assert.Equal(t, "Button", results[0].Component.Name)
	assert.Equal(t, "name", results[0].MatchReason)
}

func TestSearchComponents_ByDescription(t *testing.T) {
	qs := testQueryService()
	results := qs.SearchComponents("modal")
	require.Len(t, results, 1)
	assert.Equal(t, "Dialog", results[0].Component.Name)
	assert.Equal(t, "description", results[0].MatchReason)
}

func TestSearchComponents_ByPropName(t *testing.T) {
	qs := testQueryService()
	results := qs.SearchComponents("variant")
	require.Len(t, results, 1)
	assert.Equal(t, "Button", results[0].Component.Name)
	assert.Equal(t, "prop:variant", results[0].MatchReason)
}

func TestSearchComponents_BySubComponentName(t *testing.T) {
	qs := testQueryService()
	results := qs.SearchComponents("DialogTrigger")
	require.Len(t, results, 1)
	assert.Equal(t, "Dialog", results[0].Component.Name)
	assert.Contains(t, results[0].MatchReason, "sub-component:")
}

func TestSearchComponents_NoMatch(t *testing.T) {
	qs := testQueryService()
	results := qs.SearchComponents("zzz_nonexistent")
	assert.Empty(t, results)
}

func TestSearchComponents_EmptyQuery(t *testing.T) {
	qs := testQueryService()
	results := qs.SearchComponents("")
	assert.Nil(t, results)
}

// --- LoadAndQuery integration ---

func TestLoadAndQuery_ShadcnCatalog(t *testing.T) {
	catalogPath := filepath.Join("..", "..", "catalogs", "shadcn", "catalog.json")
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		t.Skip("shadcn catalog not found at", catalogPath)
	}

	qs, err := LoadAndQuery(catalogPath)
	require.NoError(t, err)

	// ListCategories
	cats := qs.ListCategories()
	assert.Len(t, cats, 7)

	// ListComponents by category
	overlayComps := qs.ListComponents("overlay", "")
	assert.Greater(t, len(overlayComps), 0)

	// GetComponent
	button, ok := qs.GetComponent("Button")
	require.True(t, ok)
	assert.Equal(t, "Button", button.Name)

	// GetComponent via sub-component name
	parent, ok := qs.GetComponent("DialogContent")
	require.True(t, ok)
	assert.Equal(t, "Dialog", parent.Name)

	// GetTokens by category
	colorTokens := qs.GetTokens("color")
	assert.Greater(t, len(colorTokens), 0)

	// SearchComponents
	results := qs.SearchComponents("button")
	assert.Greater(t, len(results), 0)
}

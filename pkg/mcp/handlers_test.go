package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gnana997/uispec/pkg/catalog"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/validator"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func testServer() *Server {
	cat := &catalog.Catalog{
		Name:    "test",
		Version: "1.0",
		Categories: []catalog.Category{
			{Name: "actions", Description: "Action components", Components: []string{"Button"}},
			{Name: "overlay", Components: []string{"Dialog"}},
		},
		Components: []catalog.Component{
			{
				Name:          "Button",
				Description:   "A clickable button",
				Category:      "actions",
				ImportPath:    "@/components/ui/button",
				ImportedNames: []string{"Button"},
				Props: []catalog.Prop{
					{Name: "variant", Type: "string", AllowedValues: []string{"default", "destructive"}},
					{Name: "size", Type: "string"},
				},
				Examples: []catalog.Example{
					{Title: "Basic", Code: "<Button>Click</Button>"},
				},
			},
			{
				Name:          "Dialog",
				Description:   "A modal dialog overlay",
				Category:      "overlay",
				ImportPath:    "@/components/ui/dialog",
				ImportedNames: []string{"Dialog", "DialogTrigger", "DialogContent", "DialogTitle"},
				SubComponents: []catalog.SubComponent{
					{Name: "DialogTrigger", Description: "Opens the dialog", AllowedParents: []string{"Dialog"}},
					{Name: "DialogContent", Description: "Content container", AllowedParents: []string{"Dialog"}, MustContain: []string{"DialogTitle"}},
					{Name: "DialogTitle", Description: "Accessible title", AllowedParents: []string{"DialogContent"}},
				},
				Examples: []catalog.Example{
					{Title: "Basic Dialog", Code: "<Dialog>...</Dialog>"},
				},
				Guidelines: []catalog.Guideline{
					{Rule: "dialog-title", Description: "Must have title", Severity: "error"},
				},
			},
		},
		Tokens: []catalog.Token{
			{Name: "background", Value: "hsl(var(--background))", Category: "color"},
			{Name: "foreground", Value: "hsl(var(--foreground))", Category: "color"},
			{Name: "chart-1", Value: "hsl(var(--chart-1))", Category: "chart"},
			{Name: "radius", Value: "var(--radius)", Category: "border"},
		},
		Guidelines: []catalog.Guideline{
			{Rule: "use-design-tokens", Description: "Use design tokens", Severity: "warning"},
			{Rule: "import-from-ui", Description: "Import from @/components/ui", Severity: "error"},
		},
	}

	idx := cat.BuildIndex()
	qs := catalog.NewQueryService(cat, idx)
	return NewServer(qs, nil, nil)
}

func callTool(t *testing.T, s *Server, req mcp.CallToolRequest) *mcp.CallToolResult {
	t.Helper()
	var handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)

	switch req.Params.Name {
	case "list_categories":
		handler = s.handleListCategories
	case "list_components":
		handler = s.handleListComponents
	case "get_component_details":
		handler = s.handleGetComponentDetails
	case "get_component_examples":
		handler = s.handleGetComponentExamples
	case "get_tokens":
		handler = s.handleGetTokens
	case "get_guidelines":
		handler = s.handleGetGuidelines
	case "search_components":
		handler = s.handleSearchComponents
	case "validate_page":
		handler = s.handleValidatePage
	case "analyze_page":
		handler = s.handleAnalyzePage
	default:
		t.Fatalf("unknown tool: %s", req.Params.Name)
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

func makeRequest(toolName string, args map[string]any) mcp.CallToolRequest {
	var arguments any
	if args != nil {
		arguments = args
	}
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: arguments,
		},
	}
}

func resultJSON(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content)
	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	return textContent.Text
}

// --- list_categories ---

func TestHandleListCategories(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("list_categories", nil))
	assert.False(t, result.IsError)

	var cats []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &cats))
	assert.Len(t, cats, 2)
	assert.Equal(t, "actions", cats[0]["name"])
	assert.Equal(t, float64(1), cats[0]["component_count"])
}

// --- list_components ---

func TestHandleListComponents_NoFilter(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("list_components", nil))
	assert.False(t, result.IsError)

	var comps []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &comps))
	assert.Len(t, comps, 2)
}

func TestHandleListComponents_ByCategory(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("list_components", map[string]any{"category": "actions"}))

	var comps []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &comps))
	require.Len(t, comps, 1)
	assert.Equal(t, "Button", comps[0]["name"])
}

func TestHandleListComponents_ByKeyword(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("list_components", map[string]any{"keyword": "modal"}))

	var comps []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &comps))
	require.Len(t, comps, 1)
	assert.Equal(t, "Dialog", comps[0]["name"])
}

// --- get_component_details ---

func TestHandleGetComponentDetails(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_component_details", map[string]any{
		"names": []any{"Button"},
	}))
	assert.False(t, result.IsError)

	var comps []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &comps))
	require.Len(t, comps, 1)
	assert.Equal(t, "Button", comps[0]["name"])
}

func TestHandleGetComponentDetails_SubComponentResolution(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_component_details", map[string]any{
		"names": []any{"DialogContent"},
	}))
	assert.False(t, result.IsError)

	var comps []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &comps))
	require.Len(t, comps, 1)
	assert.Equal(t, "Dialog", comps[0]["name"]) // resolves to parent
}

func TestHandleGetComponentDetails_NotFound(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_component_details", map[string]any{
		"names": []any{"NonExistent"},
	}))
	assert.True(t, result.IsError)
}

func TestHandleGetComponentDetails_MissingNames(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_component_details", nil))
	assert.True(t, result.IsError)
}

// --- get_component_examples ---

func TestHandleGetComponentExamples(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_component_examples", map[string]any{"name": "Button"}))
	assert.False(t, result.IsError)

	var examples []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &examples))
	require.Len(t, examples, 1)
	assert.Equal(t, "Basic", examples[0]["title"])
}

func TestHandleGetComponentExamples_NotFound(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_component_examples", map[string]any{"name": "NonExistent"}))
	assert.True(t, result.IsError)
}

func TestHandleGetComponentExamples_SubComponentResolves(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_component_examples", map[string]any{"name": "DialogTrigger"}))
	assert.False(t, result.IsError)

	var examples []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &examples))
	assert.Equal(t, "Basic Dialog", examples[0]["title"]) // parent's examples
}

// --- get_tokens ---

func TestHandleGetTokens_All(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_tokens", nil))
	assert.False(t, result.IsError)

	var tokens []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &tokens))
	assert.Len(t, tokens, 4)
}

func TestHandleGetTokens_ByCategory(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_tokens", map[string]any{"category": "color"}))

	var tokens []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &tokens))
	assert.Len(t, tokens, 2)
}

// --- get_guidelines ---

func TestHandleGetGuidelines_Global(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_guidelines", nil))

	var guidelines []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &guidelines))
	assert.Len(t, guidelines, 2)
}

func TestHandleGetGuidelines_WithComponent(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("get_guidelines", map[string]any{"component": "Dialog"}))

	var guidelines []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &guidelines))
	assert.Len(t, guidelines, 3) // 2 global + 1 component-specific
}

// --- search_components ---

func TestHandleSearchComponents_ByName(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("search_components", map[string]any{"query": "button"}))
	assert.False(t, result.IsError)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "Button", results[0]["name"])
	assert.Equal(t, "name", results[0]["match_reason"])
}

func TestHandleSearchComponents_NoMatch(t *testing.T) {
	s := testServer()
	result := callTool(t, s, makeRequest("search_components", map[string]any{"query": "zzz_nonexistent"}))
	assert.False(t, result.IsError)
	// returns text message, not error
	text := resultJSON(t, result)
	assert.Contains(t, text, "no components found")
}

// --- validate_page ---

func testServerWithValidator() *Server {
	cat := &catalog.Catalog{
		Name:    "test",
		Version: "1.0",
		Categories: []catalog.Category{
			{Name: "actions", Components: []string{"Button"}},
		},
		Components: []catalog.Component{
			{
				Name:          "Button",
				Category:      "actions",
				ImportPath:    "@/components/ui/button",
				ImportedNames: []string{"Button"},
				Props: []catalog.Prop{
					{Name: "variant", Type: "string", AllowedValues: []string{"default", "destructive"}, Default: "default"},
				},
			},
		},
	}
	idx := cat.BuildIndex()
	qs := catalog.NewQueryService(cat, idx)
	pm := parser.NewParserManager(nil)
	v := validator.NewValidator(cat, idx, pm)
	return NewServer(qs, v, nil)
}

func TestHandleValidatePage_Valid(t *testing.T) {
	s := testServerWithValidator()
	code := `
import { Button } from "@/components/ui/button"
export default function Page() { return <Button variant="default">Click</Button> }
`
	result := callTool(t, s, makeRequest("validate_page", map[string]any{"code": code}))
	assert.False(t, result.IsError)

	var vr map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &vr))
	assert.Equal(t, true, vr["valid"])
}

func TestHandleValidatePage_WithViolations(t *testing.T) {
	s := testServerWithValidator()
	// Missing import, invalid variant value.
	code := `export default function Page() { return <Button variant="fancy">Click</Button> }`
	result := callTool(t, s, makeRequest("validate_page", map[string]any{"code": code}))
	assert.False(t, result.IsError)

	var vr map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &vr))
	violations, ok := vr["violations"].([]any)
	require.True(t, ok)
	assert.Greater(t, len(violations), 0)
}

func TestHandleValidatePage_AutoFix(t *testing.T) {
	s := testServerWithValidator()
	code := `export default function Page() { return <Button>Click</Button> }`
	result := callTool(t, s, makeRequest("validate_page", map[string]any{
		"code":     code,
		"auto_fix": true,
	}))
	assert.False(t, result.IsError)

	var vr map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &vr))
	fixedCode, ok := vr["fixed_code"].(string)
	require.True(t, ok)
	assert.Contains(t, fixedCode, `import { Button } from "@/components/ui/button"`)
}

func TestHandleValidatePage_NoValidator(t *testing.T) {
	s := testServer() // no validator
	result := callTool(t, s, makeRequest("validate_page", map[string]any{"code": "<Button />"}))
	assert.True(t, result.IsError)
}

// --- analyze_page ---

func TestHandleAnalyzePage(t *testing.T) {
	s := testServerWithValidator()
	code := `
import { Button } from "@/components/ui/button"
export default function Page() { return <Button variant="default">Click</Button> }
`
	result := callTool(t, s, makeRequest("analyze_page", map[string]any{"code": code}))
	assert.False(t, result.IsError)

	var analysis map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultJSON(t, result)), &analysis))
	comps, ok := analysis["components"].([]any)
	require.True(t, ok)
	assert.Greater(t, len(comps), 0)
}

func TestHandleAnalyzePage_NoValidator(t *testing.T) {
	s := testServer() // no validator
	result := callTool(t, s, makeRequest("analyze_page", map[string]any{"code": "<div />"}))
	assert.True(t, result.IsError)
}

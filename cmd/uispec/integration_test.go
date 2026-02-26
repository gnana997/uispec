package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// binaryPath is set by TestMain after building the binary.
var binaryPath string

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION") == "" {
		// Run non-integration tests normally.
		os.Exit(m.Run())
	}

	// Build the binary once for all integration tests.
	tmp, err := os.MkdirTemp("", "uispec-integration-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "uispec")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = filepath.Join(".", ".") // cmd/uispec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build binary: " + err.Error())
	}

	os.Exit(m.Run())
}

// --- helpers ---

func skipIfNotIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run integration tests")
	}
}

// startServer launches uispec serve as a subprocess and returns an initialized MCP client.
func startServer(t *testing.T) *client.Client {
	t.Helper()

	c, err := client.NewStdioMCPClient(binaryPath, nil, "serve")
	require.NoError(t, err, "failed to start MCP server")

	t.Cleanup(func() {
		c.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "uispec-integration-test",
		Version: "1.0.0",
	}

	result, err := c.Initialize(ctx, initReq)
	require.NoError(t, err, "failed to initialize MCP session")
	assert.Equal(t, "uispec", result.ServerInfo.Name)

	return c
}

func callToolHelper(t *testing.T, c *client.Client, toolName string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	if args != nil {
		req.Params.Arguments = args
	}

	result, err := c.CallTool(ctx, req)
	require.NoError(t, err, "CallTool(%s) failed", toolName)
	return result
}

func extractJSON(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "expected content in result")
	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	return textContent.Text
}

// --- integration tests ---

func TestIntegration_ListTools(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tools, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)

	toolNames := make([]string, len(tools.Tools))
	for i, tool := range tools.Tools {
		toolNames[i] = tool.Name
	}

	expected := []string{
		"list_categories",
		"list_components",
		"get_component_details",
		"get_component_examples",
		"get_tokens",
		"get_guidelines",
		"search_components",
		"validate_page",
		"analyze_page",
	}
	for _, name := range expected {
		assert.Contains(t, toolNames, name, "missing tool: %s", name)
	}
}

func TestIntegration_ListCategories(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	result := callToolHelper(t, c, "list_categories", nil)
	assert.False(t, result.IsError)

	var cats []map[string]any
	require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &cats))
	assert.Greater(t, len(cats), 0, "expected at least one category")

	// Verify category structure.
	first := cats[0]
	assert.Contains(t, first, "name")
	assert.Contains(t, first, "component_count")
	assert.Contains(t, first, "components")
}

func TestIntegration_ListComponents(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	t.Run("no filter returns all", func(t *testing.T) {
		result := callToolHelper(t, c, "list_components", nil)
		assert.False(t, result.IsError)

		var comps []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &comps))
		assert.Greater(t, len(comps), 0)
	})

	t.Run("filter by category", func(t *testing.T) {
		result := callToolHelper(t, c, "list_components", map[string]any{"category": "actions"})
		assert.False(t, result.IsError)

		var comps []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &comps))
		assert.Greater(t, len(comps), 0)
		for _, comp := range comps {
			assert.Equal(t, "actions", comp["category"])
		}
	})

	t.Run("filter by keyword", func(t *testing.T) {
		result := callToolHelper(t, c, "list_components", map[string]any{"keyword": "button"})
		assert.False(t, result.IsError)

		var comps []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &comps))
		assert.Greater(t, len(comps), 0)
	})
}

func TestIntegration_GetComponentDetails(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	t.Run("existing component", func(t *testing.T) {
		result := callToolHelper(t, c, "get_component_details", map[string]any{
			"names": []any{"Button"},
		})
		assert.False(t, result.IsError)

		var comps []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &comps))
		require.Len(t, comps, 1)
		assert.Equal(t, "Button", comps[0]["name"])
		assert.Contains(t, comps[0], "props")
		assert.Contains(t, comps[0], "import_path")
	})

	t.Run("sub-component resolves to parent", func(t *testing.T) {
		result := callToolHelper(t, c, "get_component_details", map[string]any{
			"names": []any{"DialogContent"},
		})
		assert.False(t, result.IsError)

		var comps []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &comps))
		require.Len(t, comps, 1)
		assert.Equal(t, "Dialog", comps[0]["name"])
	})

	t.Run("not found returns error", func(t *testing.T) {
		result := callToolHelper(t, c, "get_component_details", map[string]any{
			"names": []any{"NonExistentComponent"},
		})
		assert.True(t, result.IsError)
	})
}

func TestIntegration_GetComponentExamples(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	result := callToolHelper(t, c, "get_component_examples", map[string]any{"name": "Button"})
	assert.False(t, result.IsError)

	var examples []map[string]any
	require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &examples))
	assert.Greater(t, len(examples), 0)
	assert.Contains(t, examples[0], "title")
	assert.Contains(t, examples[0], "code")
}

func TestIntegration_GetTokens(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	t.Run("all tokens", func(t *testing.T) {
		result := callToolHelper(t, c, "get_tokens", nil)
		assert.False(t, result.IsError)

		var tokens []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &tokens))
		assert.Greater(t, len(tokens), 0)
	})

	t.Run("filter by category", func(t *testing.T) {
		result := callToolHelper(t, c, "get_tokens", map[string]any{"category": "color"})
		assert.False(t, result.IsError)

		var tokens []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &tokens))
		assert.Greater(t, len(tokens), 0)
		for _, tok := range tokens {
			assert.Equal(t, "color", tok["category"])
		}
	})
}

func TestIntegration_GetGuidelines(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	t.Run("global guidelines", func(t *testing.T) {
		result := callToolHelper(t, c, "get_guidelines", nil)
		assert.False(t, result.IsError)

		var guidelines []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &guidelines))
		assert.Greater(t, len(guidelines), 0)
	})

	t.Run("component-specific guidelines", func(t *testing.T) {
		result := callToolHelper(t, c, "get_guidelines", map[string]any{"component": "Dialog"})
		assert.False(t, result.IsError)

		var guidelines []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &guidelines))
		assert.Greater(t, len(guidelines), 0)
	})
}

func TestIntegration_SearchComponents(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	t.Run("find by name", func(t *testing.T) {
		result := callToolHelper(t, c, "search_components", map[string]any{"query": "button"})
		assert.False(t, result.IsError)

		var results []map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &results))
		assert.Greater(t, len(results), 0)
		assert.Equal(t, "Button", results[0]["name"])
	})

	t.Run("no match returns text", func(t *testing.T) {
		result := callToolHelper(t, c, "search_components", map[string]any{"query": "zzz_nonexistent_xyz"})
		assert.False(t, result.IsError)

		text := extractJSON(t, result)
		assert.Contains(t, text, "no components found")
	})
}

func TestIntegration_ValidatePage(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	t.Run("valid code", func(t *testing.T) {
		code := `
import { Button } from "@/components/ui/button"
export default function Page() { return <Button variant="default">Click</Button> }
`
		result := callToolHelper(t, c, "validate_page", map[string]any{"code": code})
		assert.False(t, result.IsError)

		var vr map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &vr))
		assert.Equal(t, true, vr["valid"])
	})

	t.Run("invalid code has violations", func(t *testing.T) {
		code := `export default function Page() { return <Button variant="fancy">Click</Button> }`
		result := callToolHelper(t, c, "validate_page", map[string]any{"code": code})
		assert.False(t, result.IsError)

		var vr map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &vr))
		violations, ok := vr["violations"].([]any)
		require.True(t, ok)
		assert.Greater(t, len(violations), 0)
	})

	t.Run("auto_fix adds import", func(t *testing.T) {
		code := `export default function Page() { return <Button>Click</Button> }`
		result := callToolHelper(t, c, "validate_page", map[string]any{
			"code":     code,
			"auto_fix": true,
		})
		assert.False(t, result.IsError)

		var vr map[string]any
		require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &vr))
		fixedCode, ok := vr["fixed_code"].(string)
		require.True(t, ok)
		assert.Contains(t, fixedCode, `import { Button } from "@/components/ui/button"`)
	})
}

func TestIntegration_AnalyzePage(t *testing.T) {
	skipIfNotIntegration(t)
	c := startServer(t)

	code := `
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
export default function Page() {
  return (
    <Card>
      <CardContent>
        <Button variant="default">Click</Button>
      </CardContent>
    </Card>
  )
}
`
	result := callToolHelper(t, c, "analyze_page", map[string]any{"code": code})
	assert.False(t, result.IsError)

	var analysis map[string]any
	require.NoError(t, json.Unmarshal([]byte(extractJSON(t, result)), &analysis))
	comps, ok := analysis["components"].([]any)
	require.True(t, ok)
	assert.Greater(t, len(comps), 0)
}

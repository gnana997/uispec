package mcp

// ToolDefinition defines an MCP tool exposed by the server.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema interface{}
}

// RegisteredTools returns the MCP tool definitions.
func RegisteredTools() []ToolDefinition {
	return []ToolDefinition{
		{Name: "list_categories", Description: "Returns category names and component counts"},
		{Name: "list_components", Description: "Search/filter components by category or keyword"},
		{Name: "get_component_details", Description: "Full prop schemas, imports, exports (batched)"},
		{Name: "get_component_examples", Description: "Code examples (separate, opt-in)"},
		{Name: "get_tokens", Description: "Design tokens with subset filtering"},
		{Name: "get_assets", Description: "Search icons, logos, graphics by name"},
		{Name: "get_guidelines", Description: "Composition rules, accessibility requirements"},
		{Name: "get_page_template", Description: "Base template structure for generated code"},
		{Name: "validate_page", Description: "Parse code, validate against catalog, auto-fix"},
		{Name: "analyze_page", Description: "Compact structural summary for modification planning"},
		{Name: "get_file_status", Description: "Pre-computed validation from file watcher cache"},
		{Name: "get_notifications", Description: "Catalog changes, impact alerts, new component detection"},
	}
}

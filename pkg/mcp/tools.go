package mcp

import "github.com/mark3labs/mcp-go/mcp"

// listCategoriesTool returns the tool definition for list_categories.
func listCategoriesTool() mcp.Tool {
	return mcp.NewTool("list_categories",
		mcp.WithDescription("Returns all component categories with their component lists"),
	)
}

// listComponentsTool returns the tool definition for list_components.
func listComponentsTool() mcp.Tool {
	return mcp.NewTool("list_components",
		mcp.WithDescription("List components, optionally filtered by category and/or keyword"),
		mcp.WithString("category",
			mcp.Description("Filter by category name"),
		),
		mcp.WithString("keyword",
			mcp.Description("Case-insensitive search in component name and description"),
		),
	)
}

// getComponentDetailsTool returns the tool definition for get_component_details.
func getComponentDetailsTool() mcp.Tool {
	return mcp.NewTool("get_component_details",
		mcp.WithDescription("Get full details (props, sub-components, guidelines) for one or more components"),
		mcp.WithArray("names",
			mcp.Required(),
			mcp.Description("Component names to look up"),
			mcp.WithStringItems(),
		),
	)
}

// getComponentExamplesTool returns the tool definition for get_component_examples.
func getComponentExamplesTool() mcp.Tool {
	return mcp.NewTool("get_component_examples",
		mcp.WithDescription("Get code examples for a specific component"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Component name"),
		),
	)
}

// getTokensTool returns the tool definition for get_tokens.
func getTokensTool() mcp.Tool {
	return mcp.NewTool("get_tokens",
		mcp.WithDescription("Get design tokens, optionally filtered by category"),
		mcp.WithString("category",
			mcp.Description("Token category filter"),
			mcp.Enum("color", "chart", "sidebar", "border"),
		),
	)
}

// getGuidelinesTool returns the tool definition for get_guidelines.
func getGuidelinesTool() mcp.Tool {
	return mcp.NewTool("get_guidelines",
		mcp.WithDescription("Get usage guidelines. Returns global guidelines, or global + component-specific if a component name is given"),
		mcp.WithString("component",
			mcp.Description("Component name to include component-specific guidelines"),
		),
	)
}

// searchComponentsTool returns the tool definition for search_components.
func searchComponentsTool() mcp.Tool {
	return mcp.NewTool("search_components",
		mcp.WithDescription("Search components by keyword across names, descriptions, props, and sub-components"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		),
	)
}

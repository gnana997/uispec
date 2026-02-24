package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleListCategories(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	type categoryResult struct {
		Name           string   `json:"name"`
		Description    string   `json:"description,omitempty"`
		ComponentCount int      `json:"component_count"`
		Components     []string `json:"components"`
	}

	cats := s.query.ListCategories()
	results := make([]categoryResult, len(cats))
	for i, cat := range cats {
		results[i] = categoryResult{
			Name:           cat.Name,
			Description:    cat.Description,
			ComponentCount: len(cat.Components),
			Components:     cat.Components,
		}
	}
	return mcp.NewToolResultJSON(results)
}

func (s *Server) handleListComponents(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := req.GetString("category", "")
	keyword := req.GetString("keyword", "")

	type componentSummary struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		ImportPath  string `json:"import_path"`
	}

	comps := s.query.ListComponents(category, keyword)
	results := make([]componentSummary, len(comps))
	for i, c := range comps {
		results[i] = componentSummary{
			Name:        c.Name,
			Description: c.Description,
			Category:    c.Category,
			ImportPath:  c.ImportPath,
		}
	}
	return mcp.NewToolResultJSON(results)
}

func (s *Server) handleGetComponentDetails(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	names, err := req.RequireStringSlice("names")
	if err != nil {
		return mcp.NewToolResultError("names parameter is required"), nil
	}
	if len(names) == 0 {
		return mcp.NewToolResultError("names must contain at least one component name"), nil
	}

	comps := s.query.GetComponentsByNames(names)

	// Also resolve sub-component names to their parent components.
	seen := make(map[string]bool, len(comps))
	for _, c := range comps {
		seen[c.Name] = true
	}
	for _, name := range names {
		if seen[name] {
			continue
		}
		if comp, ok := s.query.GetComponent(name); ok && !seen[comp.Name] {
			seen[comp.Name] = true
			comps = append(comps, comp)
		}
	}

	if len(comps) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("no components found for names: %v", names)), nil
	}
	return mcp.NewToolResultJSON(comps)
}

func (s *Server) handleGetComponentExamples(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name parameter is required"), nil
	}

	comp, ok := s.query.GetComponent(name)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("component %q not found", name)), nil
	}

	if len(comp.Examples) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no examples available for %q", comp.Name)), nil
	}
	return mcp.NewToolResultJSON(comp.Examples)
}

func (s *Server) handleGetTokens(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := req.GetString("category", "")
	tokens := s.query.GetTokens(category)
	return mcp.NewToolResultJSON(tokens)
}

func (s *Server) handleGetGuidelines(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	component := req.GetString("component", "")
	guidelines := s.query.GetGuidelines(component)
	return mcp.NewToolResultJSON(guidelines)
}

func (s *Server) handleSearchComponents(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	results := s.query.SearchComponents(query)
	if len(results) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("no components found matching %q", query)), nil
	}

	type searchResult struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		ImportPath  string `json:"import_path"`
		MatchReason string `json:"match_reason"`
	}

	out := make([]searchResult, len(results))
	for i, r := range results {
		out[i] = searchResult{
			Name:        r.Component.Name,
			Description: r.Component.Description,
			Category:    r.Component.Category,
			ImportPath:  r.Component.ImportPath,
			MatchReason: r.MatchReason,
		}
	}
	return mcp.NewToolResultJSON(out)
}

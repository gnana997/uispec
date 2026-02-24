package mcp

import "fmt"

// HandleToolCall dispatches a tool call to the appropriate handler.
func (s *Server) HandleToolCall(toolName string, input map[string]interface{}) (interface{}, error) {
	switch toolName {
	case "list_categories":
		return nil, fmt.Errorf("not yet implemented")
	case "list_components":
		return nil, fmt.Errorf("not yet implemented")
	case "get_component_details":
		return nil, fmt.Errorf("not yet implemented")
	case "get_component_examples":
		return nil, fmt.Errorf("not yet implemented")
	case "get_tokens":
		return nil, fmt.Errorf("not yet implemented")
	case "get_assets":
		return nil, fmt.Errorf("not yet implemented")
	case "get_guidelines":
		return nil, fmt.Errorf("not yet implemented")
	case "get_page_template":
		return nil, fmt.Errorf("not yet implemented")
	case "validate_page":
		return nil, fmt.Errorf("not yet implemented")
	case "analyze_page":
		return nil, fmt.Errorf("not yet implemented")
	case "get_file_status":
		return nil, fmt.Errorf("not yet implemented")
	case "get_notifications":
		return nil, fmt.Errorf("not yet implemented")
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

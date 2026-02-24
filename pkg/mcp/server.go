package mcp

// Server implements the MCP (Model Context Protocol) server for UISpec.
type Server struct {
	// TODO: catalog, validator, transport (stdio/SSE)
}

// NewServer creates a new MCP server instance.
func NewServer() *Server {
	return &Server{}
}

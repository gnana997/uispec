package mcp

import (
	"github.com/gnana997/uispec/pkg/catalog"
	"github.com/mark3labs/mcp-go/server"
)

const serverVersion = "0.1.0-dev"

// Server implements the MCP server for UISpec, exposing catalog query tools.
type Server struct {
	mcpServer *server.MCPServer
	query     *catalog.QueryService
}

// NewServer creates a new MCP server backed by the given QueryService.
func NewServer(qs *catalog.QueryService) *Server {
	s := &Server{query: qs}

	s.mcpServer = server.NewMCPServer(
		"uispec",
		serverVersion,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	s.mcpServer.AddTools(
		server.ServerTool{Tool: listCategoriesTool(), Handler: s.handleListCategories},
		server.ServerTool{Tool: listComponentsTool(), Handler: s.handleListComponents},
		server.ServerTool{Tool: getComponentDetailsTool(), Handler: s.handleGetComponentDetails},
		server.ServerTool{Tool: getComponentExamplesTool(), Handler: s.handleGetComponentExamples},
		server.ServerTool{Tool: getTokensTool(), Handler: s.handleGetTokens},
		server.ServerTool{Tool: getGuidelinesTool(), Handler: s.handleGetGuidelines},
		server.ServerTool{Tool: searchComponentsTool(), Handler: s.handleSearchComponents},
	)

	return s
}

// ServeStdio starts the MCP server on stdin/stdout.
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcpServer)
}

package mcp

import (
	"context"
	"time"

	"github.com/gnana997/uispec/pkg/mcplog"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// loggingMiddleware returns a ToolHandlerMiddleware that records every tool
// call as a JSONL entry via the server's logger. If the logger is nil this
// method must not be called (guarded by the NewServer caller).
func (s *Server) loggingMiddleware() server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := mcplog.Now()
			result, err := next(ctx, req)
			elapsed := time.Since(start).Milliseconds()

			rb := mcplog.ResponseBytes(result)
			var errStr *string
			if err != nil {
				msg := err.Error()
				errStr = &msg
			}

			entry := mcplog.LogEntry{
				Ts:            start.UTC().Format(time.RFC3339),
				Tool:          req.Params.Name,
				Params:        mcplog.SanitizeParams(req.GetArguments()),
				DurationMs:    elapsed,
				ResponseBytes: rb,
				TokensEst:     rb / 4,
				Error:         errStr,
			}
			_ = s.logger.Write(entry)

			return result, err
		}
	}
}

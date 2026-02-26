// Package mcplog provides structured JSONL logging for MCP tool calls.
package mcplog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// LogEntry is the schema for one JSONL line written per MCP tool call.
type LogEntry struct {
	Ts            string         `json:"ts"`
	Tool          string         `json:"tool"`
	Params        map[string]any `json:"params"`
	DurationMs    int64          `json:"duration_ms"`
	ResponseBytes int            `json:"response_bytes"`
	TokensEst     int            `json:"tokens_est"`
	Error         *string        `json:"error"`
}

// Logger appends structured JSONL entries to a file.
// It is safe for concurrent use.
type Logger struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// NewLogger opens (or creates) the file at path for append-only writing.
// Parent directories are created automatically.
// Returns nil, nil if path is empty — callers treat a nil Logger as disabled.
func NewLogger(path string) (*Logger, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("mcplog: create log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("mcplog: open log file: %w", err)
	}
	return &Logger{f: f, enc: json.NewEncoder(f)}, nil
}

// Write appends a single JSONL entry. Errors are returned but are typically
// ignored by the caller so that log failures never affect tool call results.
func (l *Logger) Write(entry LogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enc.Encode(entry)
}

// Close closes the underlying log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}

// SanitizeParams returns a copy of args safe for logging.
// String values longer than shortStringMax bytes are replaced with a
// "{key}_len" integer entry so that large code/query payloads are never
// written to the log file.
func SanitizeParams(args map[string]any) map[string]any {
	const shortStringMax = 64
	out := make(map[string]any, len(args))
	for k, v := range args {
		if s, ok := v.(string); ok && len(s) > shortStringMax {
			out[k+"_len"] = len(s)
		} else {
			out[k] = v
		}
	}
	return out
}

// ResponseBytes returns the serialized byte length of a CallToolResult's
// content. Returns 0 for a nil result or on marshal error.
func ResponseBytes(result *mcp.CallToolResult) int {
	if result == nil {
		return 0
	}
	b, err := json.Marshal(result.Content)
	if err != nil {
		return 0
	}
	return len(b)
}

// Now is a replaceable clock for testing.
var Now = func() time.Time { return time.Now() }

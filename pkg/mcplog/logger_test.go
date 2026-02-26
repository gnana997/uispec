package mcplog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSanitizeParams(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		wantKeys map[string]bool // keys expected in output
		wantSkip map[string]bool // keys that should NOT appear
	}{
		{
			name:     "nil map returns empty",
			input:    nil,
			wantKeys: map[string]bool{},
		},
		{
			name:     "short string passes through",
			input:    map[string]any{"category": "actions"},
			wantKeys: map[string]bool{"category": true},
		},
		{
			name: "long string replaced with _len key",
			input: map[string]any{
				"code": string(make([]byte, 200)), // 200 bytes > 64
			},
			wantKeys: map[string]bool{"code_len": true},
			wantSkip: map[string]bool{"code": true},
		},
		{
			name: "bool and nil pass through",
			input: map[string]any{
				"auto_fix": true,
				"extra":    nil,
			},
			wantKeys: map[string]bool{"auto_fix": true, "extra": true},
		},
		{
			name: "mixed short and long strings",
			input: map[string]any{
				"name":  "Button",
				"query": string(make([]byte, 100)),
			},
			wantKeys: map[string]bool{"name": true, "query_len": true},
			wantSkip: map[string]bool{"query": true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := SanitizeParams(tc.input)
			for k := range tc.wantKeys {
				if _, ok := out[k]; !ok {
					t.Errorf("expected key %q in output", k)
				}
			}
			for k := range tc.wantSkip {
				if _, ok := out[k]; ok {
					t.Errorf("unexpected key %q in output", k)
				}
			}
		})
	}
}

func TestResponseBytes(t *testing.T) {
	t.Run("nil returns zero", func(t *testing.T) {
		if got := ResponseBytes(nil); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})
}

func TestLoggerWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	logger, err := NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	entries := []LogEntry{
		{Ts: time.Now().UTC().Format(time.RFC3339), Tool: "list_categories", Params: map[string]any{}, DurationMs: 5, ResponseBytes: 100, TokensEst: 25},
		{Ts: time.Now().UTC().Format(time.RFC3339), Tool: "validate_page", Params: map[string]any{"code_len": 1200, "auto_fix": false}, DurationMs: 42, ResponseBytes: 800, TokensEst: 200},
		{Ts: time.Now().UTC().Format(time.RFC3339), Tool: "search_components", Params: map[string]any{"query": "btn"}, DurationMs: 3, ResponseBytes: 50, TokensEst: 12},
	}

	for _, e := range entries {
		if err := logger.Write(e); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-open and read back.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var got []LogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		got = append(got, e)
	}

	if len(got) != len(entries) {
		t.Fatalf("got %d lines, want %d", len(got), len(entries))
	}
	for i, e := range entries {
		if got[i].Tool != e.Tool {
			t.Errorf("line %d: tool=%q, want %q", i, got[i].Tool, e.Tool)
		}
		if got[i].DurationMs != e.DurationMs {
			t.Errorf("line %d: duration_ms=%d, want %d", i, got[i].DurationMs, e.DurationMs)
		}
	}
}

func TestLoggerConcurrency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.jsonl")

	logger, err := NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	const goroutines = 50
	const writesEach = 10

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesEach; j++ {
				_ = logger.Write(LogEntry{
					Ts:   time.Now().UTC().Format(time.RFC3339),
					Tool: "list_categories",
				})
			}
		}(i)
	}
	wg.Wait()

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("torn write detected at line %d: %v", count+1, err)
		}
		count++
	}

	if count != goroutines*writesEach {
		t.Errorf("got %d lines, want %d", count, goroutines*writesEach)
	}
}

func TestNewLoggerCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "mcp.jsonl")

	logger, err := NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

func TestNewLoggerEmptyPath(t *testing.T) {
	logger, err := NewLogger("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logger != nil {
		t.Errorf("expected nil logger for empty path")
	}
}

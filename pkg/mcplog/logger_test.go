package mcplog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				assert.Contains(t, out, k, "expected key %q in output", k)
			}
			for k := range tc.wantSkip {
				assert.NotContains(t, out, k, "unexpected key %q in output", k)
			}
		})
	}
}

func TestResponseBytes(t *testing.T) {
	t.Run("nil returns zero", func(t *testing.T) {
		assert.Equal(t, 0, ResponseBytes(nil))
	})
}

func TestLoggerWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	logger, err := NewLogger(path)
	require.NoError(t, err)
	defer logger.Close()

	entries := []LogEntry{
		{Ts: time.Now().UTC().Format(time.RFC3339), Tool: "list_categories", Params: map[string]any{}, DurationMs: 5, ResponseBytes: 100, TokensEst: 25},
		{Ts: time.Now().UTC().Format(time.RFC3339), Tool: "validate_page", Params: map[string]any{"code_len": 1200, "auto_fix": false}, DurationMs: 42, ResponseBytes: 800, TokensEst: 200},
		{Ts: time.Now().UTC().Format(time.RFC3339), Tool: "search_components", Params: map[string]any{"query": "btn"}, DurationMs: 3, ResponseBytes: 50, TokensEst: 12},
	}

	for _, e := range entries {
		require.NoError(t, logger.Write(e))
	}

	require.NoError(t, logger.Close())

	// Re-open and read back.
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var got []LogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var e LogEntry
		require.NoError(t, json.Unmarshal([]byte(line), &e), "unmarshal line %q", line)
		got = append(got, e)
	}

	require.Len(t, got, len(entries))
	for i, e := range entries {
		assert.Equal(t, e.Tool, got[i].Tool, "line %d tool mismatch", i)
		assert.Equal(t, e.DurationMs, got[i].DurationMs, "line %d duration_ms mismatch", i)
	}
}

func TestLoggerConcurrency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.jsonl")

	logger, err := NewLogger(path)
	require.NoError(t, err)
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

	require.NoError(t, logger.Close())

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var e LogEntry
		require.NoError(t, json.Unmarshal([]byte(line), &e), "torn write detected at line %d", count+1)
		count++
	}

	assert.Equal(t, goroutines*writesEach, count)
}

func TestNewLoggerCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "mcp.jsonl")

	logger, err := NewLogger(path)
	require.NoError(t, err)
	defer logger.Close()

	_, err = os.Stat(path)
	assert.NoError(t, err, "log file should have been created")
}

func TestNewLoggerEmptyPath(t *testing.T) {
	logger, err := NewLogger("")
	require.NoError(t, err)
	assert.Nil(t, logger, "expected nil logger for empty path")
}

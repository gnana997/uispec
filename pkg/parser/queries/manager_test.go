package queries

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/parser"
)

// Test fixtures
var (
	testLogger        *slog.Logger
	testParserManager *parser.ParserManager
	testQueryManager  *QueryManager
)

// setupTest initializes test fixtures
func setupTest(t *testing.T) {
	t.Helper()

	// Create logger
	testLogger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only show errors during tests
	}))

	// Create parser manager
	testParserManager = parser.NewParserManager(testLogger)

	// Create query manager
	testQueryManager = NewQueryManager(testParserManager, testLogger)
}

// teardownTest cleans up test fixtures
func teardownTest(t *testing.T) {
	t.Helper()

	if testQueryManager != nil {
		testQueryManager.Close()
	}
	if testParserManager != nil {
		testParserManager.Close()
	}
}

// loadTestFile reads a sample file from testdata directory
func loadTestFile(t *testing.T, filename string) []byte {
	t.Helper()

	path := filepath.Join("..", "testdata", filename)
	content, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read test file %s", filename)
	return content
}

// ===========================================================================
// QUERY COMPILATION TESTS
// ===========================================================================

func TestQueryCompilation_Symbols_JavaScript(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	query, err := testQueryManager.GetQuery(parser.LanguageJavaScript, QueryTypeSymbols)
	require.NoError(t, err, "failed to compile JavaScript symbol query")
	require.NotNil(t, query, "compiled query is nil")
}

func TestQueryCompilation_Symbols_TypeScript(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	query, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryTypeSymbols)
	require.NoError(t, err, "failed to compile TypeScript symbol query")
	require.NotNil(t, query, "compiled query is nil")
}

func TestQueryCompilation_Imports_JavaScript(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	query, err := testQueryManager.GetQuery(parser.LanguageJavaScript, QueryTypeImports)
	require.NoError(t, err, "failed to compile JavaScript import query")
	require.NotNil(t, query, "compiled query is nil")
}

func TestQueryCompilation_Imports_TypeScript(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	query, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryTypeImports)
	require.NoError(t, err, "failed to compile TypeScript import query")
	require.NotNil(t, query, "compiled query is nil")
}

// ===========================================================================
// QUERY EXECUTION TESTS
// ===========================================================================

func TestQueryExecution_Symbols_TypeScript(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	// Load sample TypeScript file
	source := loadTestFile(t, "sample.ts")

	// Parse the file
	tree, err := testParserManager.Parse(source, parser.LanguageTypeScript, false)
	require.NoError(t, err, "failed to parse TypeScript file")
	defer tree.Close()

	// Get and execute symbol query
	query, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryTypeSymbols)
	require.NoError(t, err, "failed to get query")

	matches, err := testQueryManager.ExecuteQuery(tree, query, source)
	require.NoError(t, err, "failed to execute query")

	// Verify we got matches (sample.ts has interface, class, functions)
	require.NotEmpty(t, matches, "expected matches, got none")

	// Look for expected symbols
	foundInterface := false
	foundClass := false
	foundFunction := false

	for _, match := range matches {
		for _, capture := range match.Captures {
			text := capture.Text
			if text == "User" && capture.Category == "interface" {
				foundInterface = true
			}
			if text == "UserService" && capture.Category == "class" {
				foundClass = true
			}
			if text == "getUserById" && capture.Category == "function" {
				foundFunction = true
			}
		}
	}

	assert.True(t, foundInterface, "did not find User interface")
	assert.True(t, foundClass, "did not find UserService class")
	assert.True(t, foundFunction, "did not find getUserById function")
}

func TestQueryExecution_Imports_TypeScript(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	// Load sample TypeScript file
	source := loadTestFile(t, "sample.ts")

	// Parse the file
	tree, err := testParserManager.Parse(source, parser.LanguageTypeScript, false)
	require.NoError(t, err, "failed to parse TypeScript file")
	defer tree.Close()

	// Get and execute import query
	query, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryTypeImports)
	require.NoError(t, err, "failed to get query")

	matches, err := testQueryManager.ExecuteQuery(tree, query, source)
	require.NoError(t, err, "failed to execute query")

	// sample.ts has: export { User, getUserById, UserService };
	require.NotEmpty(t, matches, "expected export matches, got none")

	// Look for exports
	foundExport := false
	for _, match := range matches {
		for _, capture := range match.Captures {
			if capture.Category == "export" {
				foundExport = true
				break
			}
		}
	}

	assert.True(t, foundExport, "did not find exports")
}

// ===========================================================================
// CAPTURE PROCESSING TESTS
// ===========================================================================

func TestParseCaptureName(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedCategory string
		expectedField    string
	}{
		{
			name:             "dotted capture name",
			input:            "function.name",
			expectedCategory: "function",
			expectedField:    "name",
		},
		{
			name:             "simple capture name",
			input:            "package_name",
			expectedCategory: "package_name",
			expectedField:    "",
		},
		{
			name:             "nested dotted name",
			input:            "call.definition",
			expectedCategory: "call",
			expectedField:    "definition",
		},
		{
			name:             "export name",
			input:            "export.name",
			expectedCategory: "export",
			expectedField:    "name",
		},
		{
			name:             "import source",
			input:            "import.source",
			expectedCategory: "import",
			expectedField:    "source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, field := parseCaptureName(tt.input)
			assert.Equal(t, tt.expectedCategory, category)
			assert.Equal(t, tt.expectedField, field)
		})
	}
}

func TestNodeLocation(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	// Parse a simple TypeScript file
	source := []byte("const x: number = 1;\n")
	tree, err := testParserManager.Parse(source, parser.LanguageTypeScript, false)
	require.NoError(t, err, "failed to parse")
	defer tree.Close()

	// Get root node
	root := tree.RootNode()

	// Get location
	loc := nodeLocation(root)

	// Verify 1-based indexing
	assert.NotZero(t, loc.StartLine, "StartLine should be 1-based")
	assert.NotZero(t, loc.StartColumn, "StartColumn should be 1-based")

	// Verify byte offsets are set
	assert.NotZero(t, loc.EndByte, "EndByte should be non-zero")
}

func TestQueryCache(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	// Get query first time (should compile)
	query1, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryTypeSymbols)
	require.NoError(t, err, "failed to get query first time")

	// Get same query second time (should hit cache)
	query2, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryTypeSymbols)
	require.NoError(t, err, "failed to get query second time")

	// Should be same pointer (cached)
	assert.Same(t, query1, query2, "expected cached query to return same pointer")
}

// ===========================================================================
// CONCURRENCY TEST
// ===========================================================================

func TestConcurrentQueryExecution(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	// Load sample files
	tsSource := loadTestFile(t, "sample.ts")
	jsSource := loadTestFile(t, "sample.js")

	// Run multiple queries concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 20)

	// Launch 10 concurrent goroutines for each language
	for i := 0; i < 10; i++ {
		// TypeScript queries
		wg.Add(1)
		go func() {
			defer wg.Done()
			tree, err := testParserManager.Parse(tsSource, parser.LanguageTypeScript, false)
			if err != nil {
				errors <- err
				return
			}
			defer tree.Close()

			query, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryTypeSymbols)
			if err != nil {
				errors <- err
				return
			}

			_, err = testQueryManager.ExecuteQuery(tree, query, tsSource)
			if err != nil {
				errors <- err
			}
		}()

		// JavaScript queries
		wg.Add(1)
		go func() {
			defer wg.Done()
			tree, err := testParserManager.Parse(jsSource, parser.LanguageJavaScript, false)
			if err != nil {
				errors <- err
				return
			}
			defer tree.Close()

			query, err := testQueryManager.GetQuery(parser.LanguageJavaScript, QueryTypeSymbols)
			if err != nil {
				errors <- err
				return
			}

			_, err = testQueryManager.ExecuteQuery(tree, query, jsSource)
			if err != nil {
				errors <- err
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		assert.NoError(t, err, "concurrent execution error")
	}
}

// ===========================================================================
// PERFORMANCE TEST
// ===========================================================================

func TestQueryExecutionPerformance(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	tests := []struct {
		name     string
		filename string
		language parser.Language
	}{
		{"TypeScript", "sample.ts", parser.LanguageTypeScript},
		{"JavaScript", "sample.js", parser.LanguageJavaScript},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load file
			source := loadTestFile(t, tt.filename)

			// Parse file
			tree, err := testParserManager.Parse(source, tt.language, false)
			require.NoError(t, err, "failed to parse")
			defer tree.Close()

			// Get query
			query, err := testQueryManager.GetQuery(tt.language, QueryTypeSymbols)
			require.NoError(t, err, "failed to get query")

			// Measure execution time
			start := time.Now()
			_, err = testQueryManager.ExecuteQuery(tree, query, source)
			duration := time.Since(start)

			require.NoError(t, err, "failed to execute query")

			// Performance target: <10ms per file
			assert.Less(t, duration, 10*time.Millisecond, "query execution too slow: %v (target: <10ms)", duration)
			t.Logf("query execution time: %v (target: <10ms)", duration)
		})
	}
}

// ===========================================================================
// ERROR HANDLING TESTS
// ===========================================================================

func TestExecuteQuery_NilTree(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	query, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryTypeSymbols)
	require.NoError(t, err, "failed to get query")

	_, err = testQueryManager.ExecuteQuery(nil, query, []byte("test"))
	assert.Error(t, err, "expected error for nil tree")
}

func TestExecuteQuery_NilQuery(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	source := []byte("const x = 1;")
	tree, err := testParserManager.Parse(source, parser.LanguageTypeScript, false)
	require.NoError(t, err, "failed to parse")
	defer tree.Close()

	_, err = testQueryManager.ExecuteQuery(tree, nil, source)
	assert.Error(t, err, "expected error for nil query")
}

func TestGetQuery_UnknownLanguage(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	_, err := testQueryManager.GetQuery(parser.LanguageUnknown, QueryTypeSymbols)
	assert.Error(t, err, "expected error for unknown language")
}

func TestGetQuery_InvalidQueryType(t *testing.T) {
	setupTest(t)
	defer teardownTest(t)

	_, err := testQueryManager.GetQuery(parser.LanguageTypeScript, QueryType(999))
	assert.Error(t, err, "expected error for invalid query type")
}

package parser

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTypeScript(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewParserManager(logger)
	defer manager.Close()

	source := readTestFile(t, "sample.ts")
	tree, err := manager.Parse(source, LanguageTypeScript, false)
	require.NoError(t, err, "Parse should succeed")
	require.NotNil(t, tree, "Tree should not be nil")
	defer tree.Close()

	root := tree.RootNode()
	assert.NotNil(t, root, "Root node should not be nil")

	// TypeScript file should have a program node
	assert.Equal(t, "program", root.Kind(), "Root should be a program node")
}

func TestParseTSX(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewParserManager(logger)
	defer manager.Close()

	source := readTestFile(t, "sample.tsx")
	tree, err := manager.Parse(source, LanguageTypeScript, true)
	require.NoError(t, err, "Parse should succeed")
	require.NotNil(t, tree, "Tree should not be nil")
	defer tree.Close()

	root := tree.RootNode()
	assert.NotNil(t, root, "Root node should not be nil")
	assert.Equal(t, "program", root.Kind(), "Root should be a program node")

	// TSX should parse JSX elements
	treeString := root.ToSexp()
	assert.Contains(t, treeString, "jsx_element", "Should contain JSX elements")
}

func TestParseJavaScript(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewParserManager(logger)
	defer manager.Close()

	source := readTestFile(t, "sample.js")
	tree, err := manager.Parse(source, LanguageJavaScript, false)
	require.NoError(t, err, "Parse should succeed")
	require.NotNil(t, tree, "Tree should not be nil")
	defer tree.Close()

	root := tree.RootNode()
	assert.Equal(t, "program", root.Kind(), "Root should be a program node")
}

func TestParseFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewParserManager(logger)
	defer manager.Close()

	testCases := []struct {
		fileName     string
		expectedKind string
	}{
		{"sample.ts", "program"},
		{"sample.tsx", "program"},
		{"sample.js", "program"},
	}

	for _, tc := range testCases {
		t.Run(tc.fileName, func(t *testing.T) {
			source := readTestFile(t, tc.fileName)
			tree, err := manager.ParseFile(source, tc.fileName)
			require.NoError(t, err, "ParseFile should succeed for %s", tc.fileName)
			require.NotNil(t, tree, "Tree should not be nil")
			defer tree.Close()

			root := tree.RootNode()
			assert.Equal(t, tc.expectedKind, root.Kind(), "Root node kind should match")
		})
	}
}

func TestLazyInitialization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewParserManager(logger)
	defer manager.Close()

	// Initially, no parsers should be created
	stats := manager.GetStats()
	assert.Equal(t, 0, stats.ParsersCreated, "Should start with 0 parsers")

	// Parse TypeScript
	source := []byte("const x: number = 1;")
	tree, err := manager.Parse(source, LanguageTypeScript, false)
	require.NoError(t, err)
	require.NotNil(t, tree)
	tree.Close()

	// Now one parser should exist
	stats = manager.GetStats()
	assert.Equal(t, 1, stats.ParsersCreated, "Should have created 1 parser")
	assert.Equal(t, 1, stats.ParsesCalled, "Should have called Parse once")

	// Parse TypeScript again - should reuse parser
	tree, err = manager.Parse(source, LanguageTypeScript, false)
	require.NoError(t, err)
	require.NotNil(t, tree)
	tree.Close()

	stats = manager.GetStats()
	assert.Equal(t, 1, stats.ParsersCreated, "Should still have 1 parser (reused)")
	assert.Equal(t, 2, stats.ParsesCalled, "Should have called Parse twice")

	// Parse JavaScript - should create new parser
	tree, err = manager.Parse([]byte("const y = 2;"), LanguageJavaScript, false)
	require.NoError(t, err)
	require.NotNil(t, tree)
	tree.Close()

	stats = manager.GetStats()
	assert.Equal(t, 2, stats.ParsersCreated, "Should have created 2 parsers")
	assert.Equal(t, 3, stats.ParsesCalled, "Should have called Parse 3 times")
}

func TestLanguageDetection(t *testing.T) {
	testCases := []struct {
		filePath string
		expected Language
	}{
		{"file.ts", LanguageTypeScript},
		{"file.tsx", LanguageTypeScript},
		{"file.js", LanguageJavaScript},
		{"file.jsx", LanguageJavaScript},
		{"file.mjs", LanguageJavaScript},
		{"file.cjs", LanguageJavaScript},
		{"file.txt", LanguageUnknown},
		{"file.md", LanguageUnknown},
	}

	for _, tc := range testCases {
		t.Run(tc.filePath, func(t *testing.T) {
			lang := DetectLanguage(tc.filePath)
			assert.Equal(t, tc.expected, lang, "Language detection should match")
		})
	}
}

func TestIsTSXFile(t *testing.T) {
	testCases := []struct {
		filePath string
		expected bool
	}{
		{"file.tsx", true},
		{"file.TSX", true}, // Case insensitive
		{"file.ts", false},
		{"file.js", false},
		{"file.jsx", false},
	}

	for _, tc := range testCases {
		t.Run(tc.filePath, func(t *testing.T) {
			result := IsTSXFile(tc.filePath)
			assert.Equal(t, tc.expected, result, "TSX detection should match")
		})
	}
}

func TestParseUnknownLanguage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewParserManager(logger)
	defer manager.Close()

	source := []byte("some random text")
	tree, err := manager.Parse(source, LanguageUnknown, false)
	assert.Error(t, err, "Should return error for unknown language")
	assert.Nil(t, tree, "Tree should be nil for unknown language")
}

func TestParseInvalidSyntax(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewParserManager(logger)
	defer manager.Close()

	// Invalid TypeScript syntax
	source := []byte("const x: = ;")
	tree, err := manager.Parse(source, LanguageTypeScript, false)
	require.NoError(t, err, "Parse should not return error even for invalid syntax")
	require.NotNil(t, tree, "Tree should not be nil")
	defer tree.Close()

	// Tree should have errors
	root := tree.RootNode()
	assert.True(t, root.HasError(), "Root should have errors for invalid syntax")
}

func TestMemoryCleanup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewParserManager(logger)

	// Create parsers for multiple languages
	source := []byte("const x = 1;")
	for _, lang := range SupportedLanguages() {
		tree, err := manager.Parse(source, lang, false)
		if err == nil && tree != nil {
			tree.Close()
		}
	}

	// Close should clean up all parser pools
	err := manager.Close()
	assert.NoError(t, err, "Close should succeed")

	// Verify pools are cleared
	assert.Empty(t, manager.pools, "Pools map should be empty after Close")
}

func TestParseLanguageString(t *testing.T) {
	testCases := []struct {
		input    string
		expected Language
	}{
		{"typescript", LanguageTypeScript},
		{"TypeScript", LanguageTypeScript},
		{"ts", LanguageTypeScript},
		{"javascript", LanguageJavaScript},
		{"js", LanguageJavaScript},
		{"unknown", LanguageUnknown},
		{"", LanguageUnknown},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			lang := ParseLanguageString(tc.input)
			assert.Equal(t, tc.expected, lang, "ParseLanguageString should match")
		})
	}
}

func TestSupportedLanguages(t *testing.T) {
	languages := SupportedLanguages()
	assert.Len(t, languages, 2, "Should have 2 supported languages")
	assert.Contains(t, languages, LanguageTypeScript)
	assert.Contains(t, languages, LanguageJavaScript)
}

func TestLanguageString(t *testing.T) {
	testCases := []struct {
		lang     Language
		expected string
	}{
		{LanguageTypeScript, "typescript"},
		{LanguageJavaScript, "javascript"},
		{LanguageUnknown, "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.lang.String()
			assert.Equal(t, tc.expected, result, "String() should match")
		})
	}
}

// Helper function to read test files
func readTestFile(t *testing.T, fileName string) []byte {
	path := filepath.Join("testdata", fileName)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "Should be able to read test file %s", fileName)
	return data
}

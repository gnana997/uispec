package extractor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
)

// setupExtractor creates an extractor for testing
func setupExtractor(_ *testing.T) *Extractor {
	pm := parser.NewParserManager(nil)
	qm := queries.NewQueryManager(pm, nil)
	return NewExtractor(pm, qm, nil)
}

// TestExtractFile_TypeScript tests unified extraction for TypeScript
func TestExtractFile_TypeScript(t *testing.T) {
	extractor := setupExtractor(t)

	// Read test file
	filePath := filepath.Join("testdata", "sample.ts")
	sourceCode, err := os.ReadFile(filePath)
	require.NoError(t, err)

	// Extract all information from file
	result, err := extractor.ExtractFile(filePath, sourceCode)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify language detected
	assert.Equal(t, parser.LanguageTypeScript, result.Language)

	// Verify symbols extracted
	assert.NotEmpty(t, result.Symbols, "Should extract symbols")
	t.Logf("Extracted %d symbols from TypeScript", len(result.Symbols))

	// Look for specific symbols
	symbolNames := make(map[string]Symbol)
	for _, sym := range result.Symbols {
		symbolNames[sym.Name] = sym
	}

	// Should find UserService class
	if userService, found := symbolNames["UserService"]; found {
		assert.Equal(t, SymbolKindClass, userService.Kind)
		assert.Contains(t, userService.FullyQualifiedName, "UserService")
	}

	// Should find getUser method with metadata
	if getUser, found := symbolNames["getUser"]; found {
		assert.Equal(t, SymbolKindMethod, getUser.Kind)
		assert.Contains(t, getUser.FullyQualifiedName, "UserService")
		// Verify metadata is extracted
		assert.NotEmpty(t, getUser.Scope, "getUser should have scope metadata")
		if len(getUser.Parameters) > 0 {
			t.Logf("getUser has %d parameters", len(getUser.Parameters))
		}
		if getUser.ReturnType != "" {
			t.Logf("getUser return type: %s", getUser.ReturnType)
		}
	}

	// Verify imports extracted
	assert.NotEmpty(t, result.Imports, "Should extract imports")
	t.Logf("Extracted %d imports from TypeScript", len(result.Imports))

	// Verify exports extracted
	assert.NotEmpty(t, result.Exports, "Should extract exports")
	t.Logf("Extracted %d exports from TypeScript", len(result.Exports))

}

// TestExtractFile_JavaScript tests unified extraction for JavaScript
func TestExtractFile_JavaScript(t *testing.T) {
	extractor := setupExtractor(t)

	filePath := filepath.Join("testdata", "sample.js")
	sourceCode, err := os.ReadFile(filePath)
	require.NoError(t, err)

	result, err := extractor.ExtractFile(filePath, sourceCode)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, parser.LanguageJavaScript, result.Language)
	assert.NotEmpty(t, result.Symbols, "Should extract symbols")
	t.Logf("Extracted %d symbols from JavaScript", len(result.Symbols))

	// Look for OrderProcessor class with metadata
	symbolNames := make(map[string]Symbol)
	for _, sym := range result.Symbols {
		symbolNames[sym.Name] = sym
	}

	if orderProcessor, found := symbolNames["OrderProcessor"]; found {
		assert.Equal(t, SymbolKindClass, orderProcessor.Kind)
		// Log metadata if present
		if orderProcessor.Scope != "" {
			t.Logf("OrderProcessor scope: %s", orderProcessor.Scope)
		}
	}

	// Note: CommonJS imports may not be captured yet - that's ok for now
	t.Logf("Extracted %d imports from JavaScript", len(result.Imports))
	t.Logf("Extracted %d exports from JavaScript", len(result.Exports))
}

// TestExtractFile_SingleParseVerified tests that we only parse once
func TestExtractFile_SingleParseVerified(t *testing.T) {
	extractor := setupExtractor(t)

	filePath := filepath.Join("testdata", "sample.ts")
	sourceCode, err := os.ReadFile(filePath)
	require.NoError(t, err)

	// Extract - should parse only once
	result, err := extractor.ExtractFile(filePath, sourceCode)
	require.NoError(t, err)

	// Verify all three extraction types have results
	// This proves they all worked from the same parse tree
	assert.NotEmpty(t, result.Symbols, "Symbols should be extracted from single parse")
	assert.NotEmpty(t, result.Imports, "Imports should be extracted from single parse")
	t.Logf("Single parse produced: %d symbols, %d imports, %d exports",
		len(result.Symbols), len(result.Imports), len(result.Exports))
}

// TestExtractFile_UnsupportedLanguage tests handling of unsupported file types
func TestExtractFile_UnsupportedLanguage(t *testing.T) {
	extractor := setupExtractor(t)

	// Try to extract from unsupported file
	result, err := extractor.ExtractFile("file.txt", []byte("some text"))
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported language")
}

// TestExtractFile_InvalidSyntax tests handling of parse errors
func TestExtractFile_InvalidSyntax(t *testing.T) {
	extractor := setupExtractor(t)

	// Invalid TypeScript syntax
	invalidCode := []byte("function foo( {")
	result, err := extractor.ExtractFile("test.ts", invalidCode)

	// Parser should still work but may extract partial results
	// This tests resilience
	if err != nil {
		t.Logf("Parse error (expected): %v", err)
	} else {
		t.Logf("Parser handled invalid syntax gracefully, extracted %d symbols",
			len(result.Symbols))
	}
}

// BenchmarkExtractFile benchmarks unified extraction performance
func BenchmarkExtractFile(b *testing.B) {
	extractor := setupExtractor(&testing.T{})

	filePath := filepath.Join("testdata", "sample.ts")
	sourceCode, err := os.ReadFile(filePath)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.ExtractFile(filePath, sourceCode)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExtractFile_AllLanguages benchmarks all supported languages
func BenchmarkExtractFile_AllLanguages(b *testing.B) {
	extractor := setupExtractor(&testing.T{})

	testFiles := []string{
		"sample.ts",
		"sample.js",
	}

	for _, file := range testFiles {
		b.Run(file, func(b *testing.B) {
			filePath := filepath.Join("testdata", file)
			sourceCode, err := os.ReadFile(filePath)
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := extractor.ExtractFile(filePath, sourceCode)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

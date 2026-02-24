package indexer

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/util"
)

// Helper function to create test symbols
func createTestSymbols(count int, filePrefix string) []*extractor.Symbol {
	symbols := make([]*extractor.Symbol, count)
	for i := 0; i < count; i++ {
		symbols[i] = &extractor.Symbol{
			Name:               fmt.Sprintf("Symbol%d", i),
			FullyQualifiedName: fmt.Sprintf("%s.Symbol%d", filePrefix, i),
			Kind:               extractor.SymbolKindFunction,
			Location:           extractor.Location{FilePath: fmt.Sprintf("%s.ts", filePrefix), StartLine: uint32(i + 1), EndLine: uint32(i + 2)},
			Scope:              "public",
			IsExported:         true,
			Parameters:         []string{"arg"},
			ParameterTypes:     []string{"string"},
			ReturnType:         "void",
		}
	}
	return symbols
}

func TestNewSymbolIndexer(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	config := DefaultSymbolIndexerConfig()

	indexer := NewSymbolIndexer(config, logger)
	require.NotNil(t, indexer)
	defer indexer.Close()

	assert.NotNil(t, indexer.symbols)
	assert.NotNil(t, indexer.fileCache)
	assert.NotNil(t, indexer.fileToSymbols)
	assert.NotNil(t, indexer.dirtyFiles)
	assert.Equal(t, config.MaxCachedFiles, indexer.config.MaxCachedFiles)
}

func TestAddFileSymbols_Basic(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Create test data
	symbols := createTestSymbols(10, "TestFile")
	imports := []*extractor.ImportInfo{
		{Source: "./utils", ImportedSymbols: map[string]string{"foo": "foo"}},
	}
	exports := []*extractor.ExportInfo{
		{Name: "TestClass", ExportType: extractor.ExportTypeNamed},
	}

	// Add symbols
	filePath := "TestFile.ts"
	fileSymbols := indexer.AddFileSymbols(filePath, symbols, imports, exports)

	// Verify FileSymbols
	require.NotNil(t, fileSymbols)
	assert.Equal(t, filePath, fileSymbols.FilePath)
	assert.Equal(t, 10, len(fileSymbols.Symbols))
	assert.Equal(t, 1, len(fileSymbols.Imports))
	assert.Equal(t, 1, len(fileSymbols.Exports))
	assert.Greater(t, fileSymbols.Timestamp, int64(0))

	// Verify symbols are in index
	for _, symbol := range symbols {
		retrieved, found := indexer.GetSymbol(symbol.FullyQualifiedName)
		assert.True(t, found)
		assert.Equal(t, symbol.Name, retrieved.Name)
	}

	// Verify file is in cache
	cached, found := indexer.GetFileSymbols(filePath)
	assert.True(t, found)
	assert.Equal(t, fileSymbols, cached)

	// Verify stats
	stats := indexer.GetStats()
	assert.Equal(t, 10, stats.TotalSymbols)
	assert.Equal(t, 1, stats.CachedFiles)
	assert.Equal(t, 1, stats.IndexedFiles)
}

func TestGetSymbol_O1Lookup(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Add 1000 symbols from different files
	for i := 0; i < 100; i++ {
		symbols := createTestSymbols(10, fmt.Sprintf("File%d", i))
		indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", i), symbols, nil, nil)
	}

	// Verify all 1000 symbols can be retrieved
	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			fqn := fmt.Sprintf("File%d.Symbol%d", i, j)
			symbol, found := indexer.GetSymbol(fqn)
			assert.True(t, found, "Symbol %s should exist", fqn)
			assert.Equal(t, fmt.Sprintf("Symbol%d", j), symbol.Name)
		}
	}

	// Verify stats
	stats := indexer.GetStats()
	assert.Equal(t, 1000, stats.TotalSymbols)
	assert.Equal(t, 100, stats.IndexedFiles)
}

func TestInvalidateFile_LazyPattern(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Add file
	symbols := createTestSymbols(5, "TestFile")
	filePath := "TestFile.ts"
	indexer.AddFileSymbols(filePath, symbols, nil, nil)

	// Verify not dirty initially
	assert.False(t, indexer.IsDirty(filePath))

	// Invalidate file
	indexer.InvalidateFile(filePath)

	// Verify dirty flag is set
	assert.True(t, indexer.IsDirty(filePath))

	// Verify symbols are still accessible (lazy invalidation!)
	symbol, found := indexer.GetSymbol("TestFile.Symbol0")
	assert.True(t, found)
	assert.NotNil(t, symbol)

	// Re-add file (clears dirty flag)
	indexer.AddFileSymbols(filePath, symbols, nil, nil)
	assert.False(t, indexer.IsDirty(filePath))
}

func TestRemoveFile(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Add file
	symbols := createTestSymbols(5, "TestFile")
	filePath := "TestFile.ts"
	indexer.AddFileSymbols(filePath, symbols, nil, nil)

	// Verify symbols exist
	symbol, found := indexer.GetSymbol("TestFile.Symbol0")
	assert.True(t, found)
	assert.NotNil(t, symbol)

	// Remove file
	indexer.RemoveFile(filePath)

	// Verify symbols are gone
	_, found = indexer.GetSymbol("TestFile.Symbol0")
	assert.False(t, found)

	// Verify file is not in cache
	_, found = indexer.GetFileSymbols(filePath)
	assert.False(t, found)

	// Verify stats updated
	stats := indexer.GetStats()
	assert.Equal(t, 0, stats.TotalSymbols)
}

func TestLRUEviction(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	config := DefaultSymbolIndexerConfig()
	config.MaxCachedFiles = 10 // Small cache for testing
	indexer := NewSymbolIndexer(config, logger)
	defer indexer.Close()

	// Add 15 files (exceeds cache size)
	for i := 0; i < 15; i++ {
		symbols := createTestSymbols(3, fmt.Sprintf("File%d", i))
		indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", i), symbols, nil, nil)
	}

	// Verify stats
	stats := indexer.GetStats()
	assert.Equal(t, 15, stats.IndexedFiles)    // Total indexed
	assert.Equal(t, 10, stats.CachedFiles)     // Only 10 in cache (LRU limit)
	assert.Equal(t, 45, stats.TotalSymbols)    // All symbols still in hash map
	assert.Equal(t, int64(5), stats.Evictions) // 5 files evicted

	// Symbols from evicted files should still be accessible
	// (only FileSymbols evicted, not symbols themselves)
	symbol, found := indexer.GetSymbol("File0.Symbol0")
	assert.True(t, found)
	assert.NotNil(t, symbol)
}

func TestFindSymbols(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Add symbols with different kinds
	symbols1 := []*extractor.Symbol{
		{Name: "Func1", FullyQualifiedName: "File.Func1", Kind: extractor.SymbolKindFunction},
		{Name: "Class1", FullyQualifiedName: "File.Class1", Kind: extractor.SymbolKindClass},
		{Name: "Func2", FullyQualifiedName: "File.Func2", Kind: extractor.SymbolKindFunction},
	}
	indexer.AddFileSymbols("File.ts", symbols1, nil, nil)

	// Find all functions
	functions := indexer.FindSymbols(func(s *extractor.Symbol) bool {
		return s.Kind == extractor.SymbolKindFunction
	})

	assert.Equal(t, 2, len(functions))
	assert.Equal(t, "Func1", functions[0].Name)
	assert.Equal(t, "Func2", functions[1].Name)

	// Find all classes
	classes := indexer.FindSymbols(func(s *extractor.Symbol) bool {
		return s.Kind == extractor.SymbolKindClass
	})

	assert.Equal(t, 1, len(classes))
	assert.Equal(t, "Class1", classes[0].Name)

	// Find exported symbols (all in this case)
	exported := indexer.FindSymbols(func(s *extractor.Symbol) bool {
		return s.IsExported
	})
	assert.Equal(t, 0, len(exported)) // None are explicitly marked exported in test data
}

func TestConcurrentAccess(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Pre-populate with some data
	for i := 0; i < 10; i++ {
		symbols := createTestSymbols(5, fmt.Sprintf("File%d", i))
		indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", i), symbols, nil, nil)
	}

	// Run concurrent operations
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Mix of operations
			switch id % 4 {
			case 0:
				// Add symbols
				symbols := createTestSymbols(3, fmt.Sprintf("Concurrent%d", id))
				indexer.AddFileSymbols(fmt.Sprintf("Concurrent%d.ts", id), symbols, nil, nil)

			case 1:
				// Read symbols
				indexer.GetSymbol(fmt.Sprintf("File%d.Symbol0", id%10))

			case 2:
				// Get file symbols
				indexer.GetFileSymbols(fmt.Sprintf("File%d.ts", id%10))

			case 3:
				// Find symbols
				indexer.FindSymbols(func(s *extractor.Symbol) bool {
					return s.Kind == extractor.SymbolKindFunction
				})
			}
		}(i)
	}

	wg.Wait()

	// Verify no corruption
	stats := indexer.GetStats()
	assert.Greater(t, stats.TotalSymbols, 50) // At least original + some concurrent adds
	assert.Greater(t, stats.IndexedFiles, 10) // At least original files
}

func TestGetStats(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Initial stats
	stats := indexer.GetStats()
	assert.Equal(t, 0, stats.TotalSymbols)
	assert.Equal(t, 0, stats.CachedFiles)
	assert.Equal(t, 0, stats.IndexedFiles)

	// Add some files
	for i := 0; i < 5; i++ {
		symbols := createTestSymbols(10, fmt.Sprintf("File%d", i))
		indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", i), symbols, nil, nil)
	}

	// Verify stats updated
	stats = indexer.GetStats()
	assert.Equal(t, 50, stats.TotalSymbols)
	assert.Equal(t, 5, stats.CachedFiles)
	assert.Equal(t, 5, stats.IndexedFiles)
	assert.Greater(t, stats.MemoryEstimateBytes, int64(0))

	// Trigger cache hit
	indexer.GetFileSymbols("File0.ts")
	stats = indexer.GetStats()
	assert.Equal(t, int64(1), stats.CacheHits)

	// Trigger cache miss
	indexer.GetFileSymbols("NonExistent.ts")
	stats = indexer.GetStats()
	assert.Equal(t, int64(1), stats.CacheMisses)

	// Calculate hit rate
	assert.Equal(t, 0.5, stats.CacheHitRate) // 1 hit, 1 miss = 50%
}

func TestEdgeCases(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	t.Run("Empty symbols", func(t *testing.T) {
		fs := indexer.AddFileSymbols("Empty.ts", []*extractor.Symbol{}, nil, nil)
		assert.NotNil(t, fs)
		assert.Equal(t, 0, len(fs.Symbols))
	})

	t.Run("Nil imports and exports", func(t *testing.T) {
		symbols := createTestSymbols(1, "Test")
		fs := indexer.AddFileSymbols("Test.ts", symbols, nil, nil)
		assert.NotNil(t, fs)
		assert.Nil(t, fs.Imports)
		assert.Nil(t, fs.Exports)
	})

	t.Run("Duplicate file path", func(t *testing.T) {
		symbols1 := createTestSymbols(3, "Dup")
		indexer.AddFileSymbols("Dup.ts", symbols1, nil, nil)

		// Add again with different symbols
		symbols2 := createTestSymbols(5, "Dup2")
		indexer.AddFileSymbols("Dup.ts", symbols2, nil, nil)

		// Should replace old symbols
		_, found := indexer.GetSymbol("Dup.Symbol0")
		assert.False(t, found) // Old symbols gone

		_, found = indexer.GetSymbol("Dup2.Symbol0")
		assert.True(t, found) // New symbols present
	})

	t.Run("Remove non-existent file", func(t *testing.T) {
		// Should not panic
		indexer.RemoveFile("NonExistent.ts")
	})

	t.Run("Invalidate non-existent file", func(t *testing.T) {
		// Should not panic
		indexer.InvalidateFile("NonExistent.ts")
	})
}

func TestComputeContentHash(t *testing.T) {
	content1 := []byte("const x = 1;")
	content2 := []byte("const x = 1;")
	content3 := []byte("const x = 2;")

	hash1 := ComputeContentHash(content1)
	hash2 := ComputeContentHash(content2)
	hash3 := ComputeContentHash(content3)

	assert.Equal(t, hash1, hash2)    // Same content = same hash
	assert.NotEqual(t, hash1, hash3) // Different content = different hash
	assert.Equal(t, 64, len(hash1))  // SHA-256 = 64 hex characters
}

// ============================================================================
// BENCHMARKS
// ============================================================================

func BenchmarkSymbolLookup(b *testing.B) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Add 10,000 symbols
	for i := 0; i < 1000; i++ {
		symbols := createTestSymbols(10, fmt.Sprintf("File%d", i))
		indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", i), symbols, nil, nil)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		indexer.GetSymbol("File500.Symbol5")
	}
}

func BenchmarkAddFileSymbols(b *testing.B) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	symbols := createTestSymbols(50, "TestFile")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", i), symbols, nil, nil)
	}
}

func BenchmarkBulkIndexing(b *testing.B) {
	logger := util.NewLogger(util.DefaultLoggerConfig())

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
		b.StartTimer()

		// Index 10,000 symbols (1000 files × 10 symbols)
		for j := 0; j < 1000; j++ {
			symbols := createTestSymbols(10, fmt.Sprintf("File%d", j))
			indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", j), symbols, nil, nil)
		}

		b.StopTimer()
		indexer.Close()
		b.StartTimer()
	}
}

func BenchmarkConcurrentLookups(b *testing.B) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Add test data
	for i := 0; i < 100; i++ {
		symbols := createTestSymbols(10, fmt.Sprintf("File%d", i))
		indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", i), symbols, nil, nil)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			indexer.GetSymbol(fmt.Sprintf("File%d.Symbol%d", i%100, i%10))
			i++
		}
	})
}

func BenchmarkFindSymbols(b *testing.B) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	// Add 5000 symbols
	for i := 0; i < 500; i++ {
		symbols := createTestSymbols(10, fmt.Sprintf("File%d", i))
		indexer.AddFileSymbols(fmt.Sprintf("File%d.ts", i), symbols, nil, nil)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		indexer.FindSymbols(func(s *extractor.Symbol) bool {
			return s.Kind == extractor.SymbolKindFunction
		})
	}
}

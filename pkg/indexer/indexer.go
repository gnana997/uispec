package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/gnana997/uispec/pkg/extractor"
)

// SymbolIndexer provides O(1) symbol lookups with lazy invalidation.
//
// **Architecture:**
//   - Hash map for O(1) symbol lookups by FQN
//   - LRU cache for automatic memory management
//   - Lazy invalidation (Salsa pattern) for efficiency
//   - Reverse index for efficient file removal
//
// **Thread Safety:**
//   - Uses sync.RWMutex for concurrent access
//   - Multiple readers, single writer pattern
//   - Atomic counters for statistics
//
// **Usage:**
//
//	config := DefaultSymbolIndexerConfig()
//	indexer := NewSymbolIndexer(config, logger)
//
//	// Add symbols from extractor
//	indexer.AddFileSymbols(filePath, symbols, imports, exports)
//
//	// O(1) lookup
//	symbol, found := indexer.GetSymbol("MyClass.myMethod")
//
//	// Cleanup
//	defer indexer.Close()
type SymbolIndexer struct {
	// Primary storage: FQN → Symbol (O(1) lookups)
	symbols map[string]*extractor.Symbol

	// LRU cache: FilePath → FileSymbols
	// Automatically evicts least recently used files
	fileCache *lru.Cache[string, *FileSymbols]

	// Reverse index: FilePath → []FQN
	// Enables efficient cleanup when file is removed
	fileToSymbols map[string][]string

	// Lazy invalidation tracking: FilePath → isDirty
	dirtyFiles map[string]bool

	// Thread safety
	mu sync.RWMutex

	// Statistics (atomic for lock-free reads)
	indexedFiles   atomic.Int64
	cacheHits      atomic.Int64
	cacheMisses    atomic.Int64
	evictions      atomic.Int64
	totalIndexTime atomic.Int64 // Microseconds

	// Configuration
	config SymbolIndexerConfig

	// Logger
	logger *slog.Logger
}

// NewSymbolIndexer creates a new symbol indexer.
//
// The indexer is ready to use immediately. Call Close() when done
// to release resources.
func NewSymbolIndexer(config SymbolIndexerConfig, logger *slog.Logger) *SymbolIndexer {
	// Apply defaults for zero values
	if config.MaxCachedFiles == 0 {
		config.MaxCachedFiles = 1000
	}

	// Create LRU cache with eviction callback
	cache, err := lru.NewWithEvict(config.MaxCachedFiles, func(key string, value *FileSymbols) {
		if config.Debug {
			logger.Debug("LRU evicting file", "path", key, "symbols", len(value.Symbols))
		}
	})
	if err != nil {
		// This should never happen with valid MaxCachedFiles
		panic(fmt.Sprintf("failed to create LRU cache: %v", err))
	}

	si := &SymbolIndexer{
		symbols:       make(map[string]*extractor.Symbol, 10000), // Pre-allocate for 10k symbols
		fileCache:     cache,
		fileToSymbols: make(map[string][]string, 1000),
		dirtyFiles:    make(map[string]bool, 100),
		config:        config,
		logger:        logger,
	}

	logger.Info("SymbolIndexer initialized", "max_cached_files", config.MaxCachedFiles)
	return si
}

// AddFileSymbols adds symbols from a file to the index.
//
// This is the primary way to populate the index with results from
// the extractor. All symbols are indexed by their FQN for O(1) lookups.
//
// **Performance:** O(n) where n is number of symbols in file.
// Typical file: 10-50 symbols, <1ms overhead.
//
// **Thread Safety:** Safe for concurrent calls.
func (si *SymbolIndexer) AddFileSymbols(
	filePath string,
	symbols []*extractor.Symbol,
	imports []*extractor.ImportInfo,
	exports []*extractor.ExportInfo,
) *FileSymbols {
	return si.AddFileSymbolsWithTypes(filePath, symbols, imports, exports, nil)
}

// AddFileSymbolsWithTypes adds symbols, imports, exports AND type annotations to the index.
//
// This variant accepts type annotations for TypeScript/JavaScript files.
//
// Parameters:
//   - filePath: Absolute path to the file
//   - symbols: Extracted symbols
//   - imports: Extracted imports
//   - exports: Extracted exports
//   - typeAnnotations: Variable/parameter name → type name mappings (can be nil)
//
// Returns:
//   - FileSymbols: The indexed file data
func (si *SymbolIndexer) AddFileSymbolsWithTypes(
	filePath string,
	symbols []*extractor.Symbol,
	imports []*extractor.ImportInfo,
	exports []*extractor.ExportInfo,
	typeAnnotations map[string]string,
) *FileSymbols {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start).Microseconds()
		si.totalIndexTime.Add(elapsed)
	}()

	si.mu.Lock()
	defer si.mu.Unlock()

	// Remove old symbols for this file (if any)
	si.removeFileSymbolsUnsafe(filePath)

	// Create FileSymbols entry
	fileSymbols := &FileSymbols{
		FilePath:        filePath,
		Symbols:         symbols,
		Imports:         imports,
		Exports:         exports,
		TypeAnnotations: typeAnnotations,
		Timestamp:       time.Now().UnixMilli(),
		TokenCount:      estimateTokenCount(symbols),
	}

	// Add symbols to primary hash map
	fqns := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		si.symbols[symbol.FullyQualifiedName] = symbol
		fqns = append(fqns, symbol.FullyQualifiedName)
	}

	// Update reverse index
	si.fileToSymbols[filePath] = fqns

	// Add to LRU cache
	evicted := si.fileCache.Add(filePath, fileSymbols)
	if evicted {
		si.evictions.Add(1)
	}

	// Clear dirty flag
	delete(si.dirtyFiles, filePath)

	// Update stats
	si.indexedFiles.Add(1)

	if si.config.Debug {
		si.logger.Debug("Indexed file", "path", filePath, "symbols", len(symbols), "imports", len(imports), "exports", len(exports))
	}

	return fileSymbols
}

// GetSymbol retrieves a symbol by its fully qualified name.
//
// **Performance:** O(1) hash map lookup, typically <1ms.
//
// **Thread Safety:** Safe for concurrent calls.
//
// **Returns:**
//   - symbol: The symbol if found
//   - found: true if symbol exists in index
func (si *SymbolIndexer) GetSymbol(fullyQualifiedName string) (*extractor.Symbol, bool) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	symbol, found := si.symbols[fullyQualifiedName]
	return symbol, found
}

// GetFileSymbols retrieves all symbols for a file.
//
// **Performance:** O(1) cache lookup if file is in LRU cache.
//
// **Thread Safety:** Safe for concurrent calls.
//
// **Returns:**
//   - fileSymbols: Complete file data including symbols, imports, exports
//   - found: true if file is in cache
func (si *SymbolIndexer) GetFileSymbols(filePath string) (*FileSymbols, bool) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	fileSymbols, found := si.fileCache.Get(filePath)
	if found {
		si.cacheHits.Add(1)
	} else {
		si.cacheMisses.Add(1)
	}

	return fileSymbols, found
}

// GetAllFileSymbols returns all cached files.
//
// **Use Case:** Iteration over all indexed files.
//
// **Performance:** O(n) where n is number of cached files.
//
// **Thread Safety:** Safe for concurrent calls. Returns a snapshot.
func (si *SymbolIndexer) GetAllFileSymbols() []*FileSymbols {
	si.mu.RLock()
	defer si.mu.RUnlock()

	keys := si.fileCache.Keys()
	result := make([]*FileSymbols, 0, len(keys))

	for _, key := range keys {
		if fs, ok := si.fileCache.Peek(key); ok {
			result = append(result, fs)
		}
	}

	return result
}

// FindSymbols searches for symbols matching a predicate.
//
// **Performance:** O(n) where n is total number of symbols.
// For large codebases (10k+ symbols), consider caching results.
//
// **Thread Safety:** Safe for concurrent calls.
//
// **Example:**
//
//	// Find all functions
//	functions := indexer.FindSymbols(func(s *extractor.Symbol) bool {
//	    return s.Kind == extractor.SymbolKindFunction
//	})
//
//	// Find exported symbols
//	exported := indexer.FindSymbols(func(s *extractor.Symbol) bool {
//	    return s.IsExported
//	})
func (si *SymbolIndexer) FindSymbols(predicate func(*extractor.Symbol) bool) []*extractor.Symbol {
	si.mu.RLock()
	defer si.mu.RUnlock()

	result := make([]*extractor.Symbol, 0, 100) // Pre-allocate reasonable size

	for _, symbol := range si.symbols {
		if predicate(symbol) {
			result = append(result, symbol)
		}
	}

	return result
}

// InvalidateFile marks a file as dirty for lazy recomputation.
//
// **Lazy Invalidation (Salsa Pattern):**
//   - Does NOT immediately remove symbols
//   - Marks file as dirty (O(1) operation)
//   - Caller can detect dirty state and reindex if needed
//
// **Use Case:** File watcher detects change, marks file dirty,
// but defers reindexing until file is accessed.
//
// **Thread Safety:** Safe for concurrent calls.
func (si *SymbolIndexer) InvalidateFile(filePath string) {
	si.mu.Lock()
	si.dirtyFiles[filePath] = true
	si.mu.Unlock()

	if si.config.Debug {
		si.logger.Debug("Invalidated file", "path", filePath)
	}
}

// IsDirty checks if a file is marked for recomputation.
//
// **Thread Safety:** Safe for concurrent calls.
func (si *SymbolIndexer) IsDirty(filePath string) bool {
	si.mu.RLock()
	defer si.mu.RUnlock()

	return si.dirtyFiles[filePath]
}

// RemoveFile completely removes a file and its symbols from the index.
//
// **Performance:** O(n) where n is number of symbols in file.
// Typical file: 10-50 symbols, <1ms.
//
// **Thread Safety:** Safe for concurrent calls.
func (si *SymbolIndexer) RemoveFile(filePath string) {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.removeFileSymbolsUnsafe(filePath)

	if si.config.Debug {
		si.logger.Debug("Removed file", "path", filePath)
	}
}

// removeFileSymbolsUnsafe removes symbols for a file.
//
// **IMPORTANT:** Must be called with write lock held.
func (si *SymbolIndexer) removeFileSymbolsUnsafe(filePath string) {
	// Remove from LRU cache
	si.fileCache.Remove(filePath)

	// Remove symbols from hash map
	if fqns, exists := si.fileToSymbols[filePath]; exists {
		for _, fqn := range fqns {
			delete(si.symbols, fqn)
		}
		delete(si.fileToSymbols, filePath)
	}

	// Clear dirty flag
	delete(si.dirtyFiles, filePath)
}

// GetStats returns current indexer statistics.
//
// **Thread Safety:** Safe for concurrent calls.
func (si *SymbolIndexer) GetStats() SymbolIndexerStats {
	si.mu.RLock()

	totalSymbols := len(si.symbols)
	cachedFiles := si.fileCache.Len()
	dirtyFiles := len(si.dirtyFiles)

	si.mu.RUnlock()

	// Calculate cache hit rate
	hits := si.cacheHits.Load()
	misses := si.cacheMisses.Load()
	totalAccesses := hits + misses
	hitRate := 0.0
	if totalAccesses > 0 {
		hitRate = float64(hits) / float64(totalAccesses)
	}

	// Calculate average index time
	totalTime := si.totalIndexTime.Load()
	indexedCount := si.indexedFiles.Load()
	avgTime := 0.0
	if indexedCount > 0 {
		avgTime = float64(totalTime) / float64(indexedCount) / 1000.0 // Convert μs to ms
	}

	// Estimate memory usage
	// Rough estimate: 200 bytes per symbol + 500KB per cached file
	memoryEstimate := int64(totalSymbols)*200 + int64(cachedFiles)*500*1024

	return SymbolIndexerStats{
		IndexedFiles:        int(indexedCount),
		TotalSymbols:        totalSymbols,
		CachedFiles:         cachedFiles,
		DirtyFiles:          dirtyFiles,
		CacheHits:           hits,
		CacheMisses:         misses,
		CacheHitRate:        hitRate,
		Evictions:           si.evictions.Load(),
		MemoryEstimateBytes: memoryEstimate,
		AverageIndexTimeMs:  avgTime,
	}
}

// ComputeContentHash computes SHA-256 hash of file content.
//
// **Use Case:** Detect if file content actually changed.
// Useful for skipping reindexing of unchanged files.
func ComputeContentHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// estimateTokenCount provides a rough estimate of tokens in file.
//
// Heuristic: ~5 tokens per symbol (conservative estimate).
// Actual token counting would require running tokenizer.
func estimateTokenCount(symbols []*extractor.Symbol) int {
	// Count non-whitespace characters in symbol source
	totalChars := 0
	for _, symbol := range symbols {
		// Rough heuristic: count parameters and return type
		totalChars += len(symbol.Name) * 10 // Amplify name contribution
		totalChars += len(symbol.Parameters) * 50
		if symbol.ReturnType != "" {
			totalChars += 20
		}
	}

	// Convert characters to tokens (rough estimate: 1 token ≈ 4 characters)
	return totalChars / 4
}

// GetTypeAnnotation retrieves the type annotation for a variable/parameter in a file.
//
// Used for TypeScript/JavaScript method call resolution.
//
// Parameters:
//   - filePath: Absolute path to the file
//   - varName: Variable/parameter/property name
//
// Returns:
//   - typeName: The declared type (e.g., "UserService", "number")
//   - found: true if the annotation exists
//
// Example:
//
//	typeName, found := indexer.GetTypeAnnotation("file.ts", "service")
//	if found {
//	    // typeName might be "UserService"
//	    fqn := typeName + ".getUser"
//	}
//
// Performance: O(1) lookup
// Thread Safety: Safe for concurrent calls
func (si *SymbolIndexer) GetTypeAnnotation(filePath, varName string) (string, bool) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	// Get file from cache
	fileSymbols, found := si.fileCache.Get(filePath)
	if !found {
		return "", false
	}

	// Look up type annotation
	if fileSymbols.TypeAnnotations == nil {
		return "", false
	}

	typeName, ok := fileSymbols.TypeAnnotations[varName]
	return typeName, ok
}

// Close releases all resources held by the indexer.
//
// **IMPORTANT:** Indexer cannot be used after calling Close().
//
// **Thread Safety:** Safe to call, but caller must ensure no
// concurrent operations are in progress.
func (si *SymbolIndexer) Close() {
	si.mu.Lock()
	defer si.mu.Unlock()

	// Clear all data structures
	si.symbols = nil
	si.fileCache.Purge()
	si.fileToSymbols = nil
	si.dirtyFiles = nil

	si.logger.Info("SymbolIndexer closed")
}

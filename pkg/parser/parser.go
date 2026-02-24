package parser

import (
	"fmt"
	"log/slog"
	"sync"
	"unsafe"

	ts "github.com/tree-sitter/go-tree-sitter"
	ts_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	ts_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// defaultPoolSize is computed dynamically based on CPU count in pool_config.go
// Use getDefaultPoolSize() to get the current value

// poolKey uniquely identifies a parser pool (language + TSX variant)
type poolKey struct {
	lang  Language
	isTSX bool
}

// ParserManager manages tree-sitter parsers for multiple languages with
// lazy initialization and thread-safe concurrent access.
//
// Memory Management:
// - Parser pools are created lazily on first use per language
// - ParserManager owns parser pool instances and must be closed via Close()
// - Callers own Tree instances and must call tree.Close() after use
//
// Thread Safety:
// - Uses parser pools for true concurrent parsing
// - Multiple goroutines can parse the same language simultaneously
// - Pool creation is synchronized with write locks
// - Each pool contains up to 4 parsers (expandable)
//
// Example:
//
//	logger := util.NewLogger(util.DefaultLoggerConfig())
//	manager := NewParserManager(logger)
//	defer manager.Close()
//
//	tree, err := manager.Parse([]byte("const x = 1;"), LanguageJavaScript, false)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer tree.Close()
type ParserManager struct {
	// pools stores parser pools per language (lazily initialized)
	pools map[poolKey]*parserPool

	// mutex provides thread-safe access to pools map and stats
	mutex sync.RWMutex

	// logger for structured logging
	logger *slog.Logger

	// stats tracks parser usage statistics
	stats struct {
		parsersCreated int
		parsesCalled   int
	}
}

// NewParserManager creates a new ParserManager instance.
//
// The returned manager must be closed via Close() to free resources.
//
// Example:
//
//	manager := NewParserManager(logger)
//	defer manager.Close()
func NewParserManager(logger *slog.Logger) *ParserManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &ParserManager{
		pools:  make(map[poolKey]*parserPool),
		logger: logger,
	}
}

// Parse parses source code using the specified language grammar.
//
// The isTSX parameter is only relevant for TypeScript - it enables JSX support.
// For all other languages, isTSX is ignored.
//
// Returns a Tree that MUST be closed by the caller via tree.Close() to avoid memory leaks.
//
// Performance:
// - First parse per language: ~100ms (lazy initialization overhead)
// - Subsequent parses: <50ms (parser already initialized)
//
// Thread Safety:
// - Safe for concurrent use from multiple goroutines
// - Uses parser pool to allow true concurrent parsing
// - Up to 4 goroutines can parse the same language simultaneously
//
// Example:
//
//	// Parse TypeScript
//	tree, err := manager.Parse([]byte("const x: number = 1;"), LanguageTypeScript, false)
//	if err != nil {
//	    return err
//	}
//	defer tree.Close()
//
//	// Parse TSX (TypeScript with JSX)
//	tree, err = manager.Parse([]byte("<div>Hello</div>"), LanguageTypeScript, true)
//	if err != nil {
//	    return err
//	}
//	defer tree.Close()
func (pm *ParserManager) Parse(source []byte, lang Language, isTSX bool) (*ts.Tree, error) {
	if lang == LanguageUnknown {
		return nil, fmt.Errorf("cannot parse unknown language")
	}

	// Increment parse counter (protected by mutex)
	pm.mutex.Lock()
	pm.stats.parsesCalled++
	pm.mutex.Unlock()

	// Get or create pool for this language
	pool, err := pm.getOrCreatePool(lang, isTSX)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool for %s: %w", lang, err)
	}

	// Acquire a parser from the pool
	parser, err := pool.acquire()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire parser: %w", err)
	}

	// Parse the source code
	tree := parser.Parse(source, nil)

	// Release parser back to pool immediately
	pool.release(parser)

	if tree == nil {
		return nil, fmt.Errorf("parser.Parse returned nil tree")
	}

	// Log parse errors (but still return tree - partial trees are useful)
	root := tree.RootNode()
	if root.HasError() {
		pm.logger.Warn("parse tree contains errors",
			"language", lang.String(),
			"errors", true)
	}

	return tree, nil
}

// ParseFile is a convenience method that parses a file by detecting its language
// from the file path.
//
// Returns a Tree that MUST be closed by the caller via tree.Close().
//
// Example:
//
//	tree, err := manager.ParseFile([]byte(content), "src/app.ts")
//	if err != nil {
//	    return err
//	}
//	defer tree.Close()
func (pm *ParserManager) ParseFile(source []byte, filePath string) (*ts.Tree, error) {
	lang := DetectLanguage(filePath)
	if lang == LanguageUnknown {
		return nil, fmt.Errorf("unsupported file extension: %s", filePath)
	}

	isTSX := IsTSXFile(filePath)
	return pm.Parse(source, lang, isTSX)
}

// Close releases all parser pool resources.
//
// MUST be called when ParserManager is no longer needed to avoid memory leaks.
// After Close(), the ParserManager cannot be used.
//
// Example:
//
//	manager := NewParserManager(logger)
//	defer manager.Close()
func (pm *ParserManager) Close() error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.logger.Info("closing ParserManager",
		"parsers_created", pm.stats.parsersCreated,
		"parses_called", pm.stats.parsesCalled)

	// Close all parser pools
	for key, pool := range pm.pools {
		if pool != nil {
			pool.close()
			pm.logger.Debug("closed parser pool",
				"language", key.lang.String(),
				"isTSX", key.isTSX)
		}
	}

	// Clear map
	pm.pools = make(map[poolKey]*parserPool)

	return nil
}

// getOrCreatePool returns an existing parser pool or creates a new one.
// Thread-safe using double-checked locking pattern.
func (pm *ParserManager) getOrCreatePool(lang Language, isTSX bool) (*parserPool, error) {
	key := poolKey{lang: lang, isTSX: isTSX}

	// Fast path: pool already exists (read lock)
	pm.mutex.RLock()
	pool, exists := pm.pools[key]
	pm.mutex.RUnlock()

	if exists {
		return pool, nil
	}

	// Slow path: create pool (write lock)
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Double-check: another goroutine may have created it
	if pool, exists = pm.pools[key]; exists {
		return pool, nil
	}

	// Get language pointer
	langPtr, err := pm.GetLanguagePointer(lang, isTSX)
	if err != nil {
		return nil, err
	}

	// Create new parser pool with CPU-aware sizing
	poolSize := getDefaultPoolSize()
	pool = newParserPool(lang, langPtr, isTSX, poolSize, pm.logger)
	pm.pools[key] = pool

	pm.logger.Debug("created new parser pool",
		"language", lang.String(),
		"isTSX", isTSX,
		"maxSize", poolSize)

	return pool, nil
}

// GetLanguagePointer returns the unsafe.Pointer to the tree-sitter language grammar.
//
// This is a public method used by QueryManager to compile queries.
// The isTSX parameter is only relevant for TypeScript (enables JSX support).
func (pm *ParserManager) GetLanguagePointer(lang Language, isTSX bool) (unsafe.Pointer, error) {
	switch lang {
	case LanguageTypeScript:
		if isTSX {
			return ts_typescript.LanguageTSX(), nil
		}
		return ts_typescript.LanguageTypescript(), nil

	case LanguageJavaScript:
		return ts_javascript.Language(), nil

	default:
		return nil, fmt.Errorf("unsupported language: %s", lang.String())
	}
}

// GetStats returns parser usage statistics.
//
// Example:
//
//	stats := manager.GetStats()
//	fmt.Printf("Parsers created: %d, Parses called: %d\n",
//	    stats.ParsersCreated, stats.ParsesCalled)
func (pm *ParserManager) GetStats() ParserStats {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// Count total parsers created across all pools
	totalParsers := 0
	for _, pool := range pm.pools {
		totalParsers += pool.getCreatedCount()
	}

	return ParserStats{
		ParsersCreated: totalParsers,
		ParsesCalled:   pm.stats.parsesCalled,
	}
}

// ParserStats contains parser usage statistics.
type ParserStats struct {
	// ParsersCreated is the total number of parser instances created
	ParsersCreated int

	// ParsesCalled is the total number of Parse() calls
	ParsesCalled int
}

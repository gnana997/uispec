// Package queries provides tree-sitter query compilation, caching, and execution.
package queries

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries/imports"
	"github.com/gnana997/uispec/pkg/parser/queries/symbols"
	"github.com/gnana997/uispec/pkg/parser/queries/types"
)

// QueryType identifies which type of query to execute (symbols, imports, types).
type QueryType int

const (
	// QueryTypeSymbols extracts symbol definitions (functions, classes, variables, etc.)
	QueryTypeSymbols QueryType = iota
	// QueryTypeImports extracts import/export statements for dependency graph construction
	QueryTypeImports
	// QueryTypeTypes extracts type annotations from TypeScript/JavaScript code
	QueryTypeTypes
)

// String returns the string representation of a QueryType.
func (qt QueryType) String() string {
	switch qt {
	case QueryTypeSymbols:
		return "symbols"
	case QueryTypeImports:
		return "imports"
	case QueryTypeTypes:
		return "types"
	default:
		return "unknown"
	}
}

// queryKey uniquely identifies a compiled query (language + type).
type queryKey struct {
	lang  parser.Language
	qtype QueryType
}

// QueryManager manages tree-sitter query compilation and caching.
//
// Features:
//   - Lazy query compilation: Queries compiled on first use
//   - Thread-safe caching: Uses sync.RWMutex for concurrent access
//   - Memory management: Queries freed via Close()
//
// Usage:
//
//	qm := NewQueryManager(parserManager, logger)
//	defer qm.Close()
//
//	// Get compiled query
//	query, err := qm.GetQuery(parser.LanguageTypeScript, QueryTypeSymbols)
//	if err != nil {
//	    return err
//	}
//
//	// Execute query
//	matches, err := qm.ExecuteQuery(tree, query, sourceCode)
//	if err != nil {
//	    return err
//	}
type QueryManager struct {
	parserManager *parser.ParserManager
	cache         map[queryKey]*ts.Query
	mutex         sync.RWMutex
	logger        *slog.Logger
}

// NewQueryManager creates a new query manager.
//
// The parserManager is required to access language-specific parsers for query compilation.
// Logger can be nil (will use default slog logger).
func NewQueryManager(pm *parser.ParserManager, logger *slog.Logger) *QueryManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &QueryManager{
		parserManager: pm,
		cache:         make(map[queryKey]*ts.Query),
		logger:        logger,
	}
}

// GetQuery returns a compiled query for the specified language and type.
//
// Queries are compiled lazily on first access and cached for subsequent calls.
// This method is thread-safe.
//
// Returns an error if:
//   - Language is unknown or unsupported
//   - Query compilation fails (invalid query syntax)
func (qm *QueryManager) GetQuery(lang parser.Language, qtype QueryType) (*ts.Query, error) {
	key := queryKey{lang: lang, qtype: qtype}

	// Fast path: Check if query already compiled (read lock)
	qm.mutex.RLock()
	query, exists := qm.cache[key]
	qm.mutex.RUnlock()

	if exists {
		return query, nil
	}

	// Slow path: Compile query (write lock)
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	// Double-check: Another goroutine may have compiled it
	if query, exists = qm.cache[key]; exists {
		return query, nil
	}

	// Get query string
	queryString, err := qm.getQueryString(lang, qtype)
	if err != nil {
		return nil, err
	}

	// Get language pointer for compilation
	langPtr, err := qm.parserManager.GetLanguagePointer(lang, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get language pointer for %s: %w", lang, err)
	}

	// Wrap language pointer
	tsLang := ts.NewLanguage(langPtr)

	// Compile query
	query, qerr := ts.NewQuery(tsLang, queryString)
	if qerr != nil {
		return nil, fmt.Errorf("failed to compile %s query for %s: %s", qtype, lang, qerr.Message)
	}

	// Cache compiled query
	qm.cache[key] = query

	qm.logger.Debug("compiled query",
		"language", lang.String(),
		"type", qtype.String())

	return query, nil
}

// getQueryString returns the query string for a language and type.
func (qm *QueryManager) getQueryString(lang parser.Language, qtype QueryType) (string, error) {
	switch qtype {
	case QueryTypeSymbols:
		return qm.getSymbolQuery(lang)
	case QueryTypeImports:
		return qm.getImportQuery(lang)
	case QueryTypeTypes:
		return qm.getTypesQuery(lang)
	default:
		return "", fmt.Errorf("unknown query type: %d", qtype)
	}
}

// getSymbolQuery returns the symbol extraction query for a language.
func (qm *QueryManager) getSymbolQuery(lang parser.Language) (string, error) {
	switch lang {
	case parser.LanguageJavaScript:
		return symbols.JSQueries, nil
	case parser.LanguageTypeScript:
		return symbols.TSQueries, nil
	default:
		return "", fmt.Errorf("unsupported language for symbol queries: %s", lang)
	}
}

// getImportQuery returns the import/export extraction query for a language.
func (qm *QueryManager) getImportQuery(lang parser.Language) (string, error) {
	switch lang {
	case parser.LanguageJavaScript:
		return imports.JSQueries, nil
	case parser.LanguageTypeScript:
		return imports.TSQueries, nil
	default:
		return "", fmt.Errorf("unsupported language for import queries: %s", lang)
	}
}

// getTypesQuery returns the type annotation extraction query for a language.
//
// Type annotations are only supported for TypeScript/JavaScript.
// Returns an error for other languages.
func (qm *QueryManager) getTypesQuery(lang parser.Language) (string, error) {
	switch lang {
	case parser.LanguageTypeScript:
		return types.TSQueries, nil
	case parser.LanguageJavaScript:
		// JavaScript can also have JSDoc type annotations
		// For now, use same TypeScript queries (they work on JS too)
		return types.TSQueries, nil
	default:
		return "", fmt.Errorf("type annotation queries not supported for language: %s", lang)
	}
}

// ExecuteQuery runs a compiled query on a parse tree and returns structured matches.
//
// Parameters:
//   - tree: The parse tree to query
//   - query: The compiled query (from GetQuery)
//   - source: The original source code (for extracting matched text)
//
// Returns:
//   - []QueryMatch: Structured query results with captures
//   - error: If query execution fails
//
// Performance: Typical execution time is <10ms per file.
func (qm *QueryManager) ExecuteQuery(tree *ts.Tree, query *ts.Query, source []byte) ([]QueryMatch, error) {
	if tree == nil {
		return nil, fmt.Errorf("tree is nil")
	}
	if query == nil {
		return nil, fmt.Errorf("query is nil")
	}

	// Create query cursor
	cursor := ts.NewQueryCursor()
	defer cursor.Close()

	// Execute query - returns iterator
	iter := cursor.Matches(query, tree.RootNode(), source)

	// Get capture names from query
	captureNames := query.CaptureNames()

	// Collect matches
	var matches []QueryMatch
	for {
		match := iter.Next()
		if match == nil {
			break
		}

		// Process captures for this match
		var captures []QueryCapture
		for _, capture := range match.Captures {
			// Get capture name from index
			var captureName string
			if int(capture.Index) < len(captureNames) {
				captureName = captureNames[capture.Index]
			}

			// Parse capture name (e.g., "function.name" → category="function", field="name")
			category, field := parseCaptureName(captureName)

			// Extract node text
			text := capture.Node.Utf8Text(source)

			// Build capture result
			captures = append(captures, QueryCapture{
				Name:     captureName,
				Category: category,
				Field:    field,
				Node:     &capture.Node,
				Text:     text,
				Location: nodeLocation(&capture.Node),
			})
		}

		matches = append(matches, QueryMatch{
			PatternIndex: uint32(match.PatternIndex),
			Captures:     captures,
		})
	}

	return matches, nil
}

// Close releases all compiled queries.
//
// MUST be called when QueryManager is no longer needed to avoid memory leaks.
// After Close(), the QueryManager cannot be used.
func (qm *QueryManager) Close() error {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	qm.logger.Info("closing QueryManager",
		"queries_compiled", len(qm.cache))

	// Delete all queries from tree-sitter
	for key, query := range qm.cache {
		if query != nil {
			query.Close()
		}
		delete(qm.cache, key)
	}

	return nil
}

// QueryMatch represents a single pattern match from query execution.
type QueryMatch struct {
	// PatternIndex identifies which query pattern matched
	PatternIndex uint32

	// Captures contains all captured nodes for this match
	Captures []QueryCapture
}

// QueryCapture represents a single captured node from a query match.
type QueryCapture struct {
	// Name is the full capture name (e.g., "function.name", "call.definition")
	Name string

	// Category is the first part of the capture name (e.g., "function", "call")
	Category string

	// Field is the second part of the capture name (e.g., "name", "definition")
	// Empty string if capture name has no dot
	Field string

	// Node is the captured AST node
	Node *ts.Node

	// Text is the source code text of the captured node
	Text string

	// Location is the file location of the captured node
	Location Location
}

// Location represents a position in source code.
type Location struct {
	StartLine   uint32 // 1-based line number
	StartColumn uint32 // 1-based column number
	EndLine     uint32
	EndColumn   uint32
	StartByte   uint32 // 0-based byte offset
	EndByte     uint32
}

// parseCaptureName splits a capture name like "function.name" into ("function", "name").
//
// If the name has no dot, returns (name, "").
// Examples:
//   - "function.name" → ("function", "name")
//   - "call.definition" → ("call", "definition")
//   - "package_name" → ("package_name", "")
func parseCaptureName(name string) (category, field string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return name, ""
}

// nodeLocation extracts location information from a tree-sitter node.
//
// Converts tree-sitter's 0-based coordinates to 1-based line/column numbers
// for consistency with LSP and most editor APIs.
func nodeLocation(node *ts.Node) Location {
	start := node.StartPosition()
	end := node.EndPosition()

	return Location{
		StartLine:   uint32(start.Row + 1),    // Convert 0-based to 1-based
		StartColumn: uint32(start.Column + 1), // Convert 0-based to 1-based
		EndLine:     uint32(end.Row + 1),
		EndColumn:   uint32(end.Column + 1),
		StartByte:   uint32(node.StartByte()),
		EndByte:     uint32(node.EndByte()),
	}
}

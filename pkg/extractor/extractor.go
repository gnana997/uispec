// Package extractor implements unified per-file extraction of symbols and imports/exports.
package extractor

import (
	"fmt"
	"log/slog"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
)

// Extractor performs unified extraction of symbols and imports/exports.
//
// Critical optimization: Parses each file ONCE and runs all extractors on
// the same AST tree.
//
// Usage:
//
//	extractor := NewExtractor(parserManager, queryManager, logger)
//	result, err := extractor.ExtractFile(filePath, sourceCode)
//	if err != nil {
//	    return err
//	}
//	// Use result.Symbols, result.Imports, result.Exports
type Extractor struct {
	parserManager *parser.ParserManager
	queryManager  *queries.QueryManager
	logger        *slog.Logger
}

// NewExtractor creates a new unified extractor.
//
// The parserManager is used to parse files once, and the queryManager is used
// to execute all three query types (symbols, calls, imports) on the same tree.
func NewExtractor(pm *parser.ParserManager, qm *queries.QueryManager, logger *slog.Logger) *Extractor {
	if logger == nil {
		logger = slog.Default()
	}

	return &Extractor{
		parserManager: pm,
		queryManager:  qm,
		logger:        logger,
	}
}

// ExtractFile parses a file ONCE and extracts ALL information from the same AST tree.
//
// This is the main entry point for unified extraction. It:
// 1. Detects language from file extension
// 2. Parses file ONCE using ParserManager
// 3. Executes all queries on same tree (symbols, imports, types)
// 4. Builds symbols with metadata
// 5. Builds imports/exports
// 6. Extracts type annotations
// 7. Closes tree (memory cleanup)
// 8. Returns PerFileResult
func (e *Extractor) ExtractFile(filePath string, sourceCode []byte) (*PerFileResult, error) {
	// 1. Detect language from file extension
	lang := parser.DetectLanguage(filePath)
	if lang == parser.LanguageUnknown {
		return nil, fmt.Errorf("unsupported language for file: %s", filePath)
	}

	// Check if file is TSX (special handling for TypeScript)
	isTSX := parser.IsTSXFile(filePath)

	// 2. Parse file ONCE using ParserManager
	tree, err := e.parserManager.Parse(sourceCode, lang, isTSX)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
	}
	defer tree.Close() // CRITICAL: Close tree after extraction to avoid memory leak

	// 3. Execute queries on same tree
	// Get queries (cached by QueryManager)
	symbolQuery, err := e.queryManager.GetQuery(lang, queries.QueryTypeSymbols, isTSX)
	if err != nil {
		return nil, fmt.Errorf("failed to get symbol query for %s: %w", lang, err)
	}

	importQuery, err := e.queryManager.GetQuery(lang, queries.QueryTypeImports, isTSX)
	if err != nil {
		return nil, fmt.Errorf("failed to get import query for %s: %w", lang, err)
	}

	// Execute queries on the same tree
	symbolMatches, err := e.queryManager.ExecuteQuery(tree, symbolQuery, sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to execute symbol query: %w", err)
	}

	importMatches, err := e.queryManager.ExecuteQuery(tree, importQuery, sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to execute import query: %w", err)
	}

	// 4. Build symbols from symbol query results (includes metadata extraction)
	symbols := e.extractSymbols(symbolMatches, tree, sourceCode, filePath, lang)

	// 5. Build imports/exports from import query results
	imports := e.extractImports(importMatches, sourceCode, filePath, lang)
	exports := e.extractExports(importMatches, sourceCode, filePath, lang)

	// 6. Extract type annotations (TypeScript/JavaScript only) - MUST come before call sites!
	// This enables method call resolution: service.getUser() → UserService.getUser()
	typeAnnotations := make(map[string]string)
	if lang == parser.LanguageTypeScript || lang == parser.LanguageJavaScript {
		typeAnnotations = e.extractTypeAnnotations(tree, sourceCode, lang, isTSX)
		// Log type annotations for debugging TypeScript type inference
		if len(typeAnnotations) > 0 {
			e.logger.Info("extracted type annotations",
				"file", filePath,
				"count", len(typeAnnotations))
			// Log first few annotations for debugging
			count := 0
			for varName, typeName := range typeAnnotations {
				if count >= 5 {
					break
				}
				e.logger.Info("type annotation", "var", varName, "type", typeName)
				count++
			}
		}
	}

	// Log extraction summary
	e.logger.Debug("extracted file",
		"file", filePath,
		"language", lang,
		"symbols", len(symbols),
		"imports", len(imports),
		"exports", len(exports),
		"typeAnnotations", len(typeAnnotations))

	// 7. Return PerFileResult (tree already closed via defer)
	return &PerFileResult{
		FilePath:        filePath,
		Language:        lang,
		Symbols:         symbols,
		Imports:         imports,
		Exports:         exports,
		TypeAnnotations: typeAnnotations,
	}, nil
}

// extractTypeAnnotations extracts TypeScript/JavaScript type annotations from the AST.
//
// Type annotations enable method call resolution by providing variable types:
//   const service: UserService = new UserService();
//   service.getUser() → resolves to UserService.getUser()
//
// Extraction strategy:
//   - Extract explicit type annotations (: TypeName)
//   - For generics, prefer first type argument (Array<User> → User)
//   - Skip unions, intersections, and conditional types (Phase 4)
//
// Returns a map: varName → typeName
//
// Performance: <5ms per file (single query execution)
func (e *Extractor) extractTypeAnnotations(tree *ts.Tree, sourceCode []byte, lang parser.Language, isTSX ...bool) map[string]string {
	annotations := make(map[string]string)

	// Get types query
	typesQuery, err := e.queryManager.GetQuery(lang, queries.QueryTypeTypes, isTSX...)
	if err != nil {
		e.logger.Debug("failed to get types query",
			"language", lang,
			"error", err)
		return annotations
	}

	// Execute query
	matches, err := e.queryManager.ExecuteQuery(tree, typesQuery, sourceCode)
	if err != nil {
		e.logger.Debug("failed to execute types query",
			"error", err)
		return annotations
	}

	// Process matches to extract varName → typeName mappings
	// Capture indices from queries/types/typescript.go:
	//   @type.var.name (index 0) - Variable/parameter/property name
	//   @type.name     (index 1) - Type name (simple types)
	//   @type.base     (index 2) - Base type for generics
	//   @type.arg      (index 3) - Type argument (preferred for generics)
	//
	// For intersection types (A & B), we may get multiple @type.name or @type.arg captures.
	// Strategy: Collect ALL types and use the first non-empty one (prefer type.arg > type.name > type.base)
	for _, match := range matches {
		varName := ""
		var typeNames []string
		var typeArgs []string
		typeBase := ""

		for _, capture := range match.Captures {
			switch capture.Name {
			case "type.var.name":
				varName = capture.Text
			case "type.name":
				// Collect all type names (for intersections like Observer & RequestOptions)
				if capture.Text != "" {
					typeNames = append(typeNames, capture.Text)
				}
			case "type.base":
				typeBase = capture.Text
			case "type.arg":
				// Collect all type arguments (for intersections like Partial<Observer> & Other<T>)
				if capture.Text != "" {
					typeArgs = append(typeArgs, capture.Text)
				}
			}
		}

		if varName == "" {
			continue
		}

		// Priority: type.arg > type.name > type.base
		// For intersection types, prefer the first type.arg (from generics) or first type.name
		// Example: Partial<TRPCSubscriptionObserver> & TRPCRequestOptions
		//   → typeArgs = ["TRPCSubscriptionObserver"], typeNames = ["TRPCRequestOptions"]
		//   → Use "TRPCSubscriptionObserver" (has the methods we care about)
		finalType := ""
		if len(typeArgs) > 0 {
			finalType = typeArgs[0] // Prefer first type argument from generics
		} else if len(typeNames) > 0 {
			finalType = typeNames[0] // Fallback to first type name
		} else if typeBase != "" {
			finalType = typeBase // Last resort: base type
		}

		if finalType != "" {
			annotations[varName] = finalType
		}
	}

	return annotations
}

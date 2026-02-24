// Package extractor provides unified per-file extraction of symbols and imports/exports.
//
// Critical optimization: Parse each file ONCE and extract ALL information from the same AST tree.
package extractor

import "github.com/gnana997/uispec/pkg/parser"

// PerFileResult contains all extracted information from a single file.
//
// Represents the complete extraction result from parsing a file once and running
// all extractors (symbols, imports/exports) on the same tree.
type PerFileResult struct {
	FilePath string
	Language parser.Language
	Symbols  []Symbol
	Imports  []ImportInfo
	Exports  []ExportInfo

	// Type annotations extracted from TypeScript/JavaScript code
	// Maps variable/parameter/property names to their declared types
	// Example: "service" → "UserService", "count" → "number"
	TypeAnnotations map[string]string
}

// Symbol represents a code symbol (function, class, method, variable, type, etc.) with full metadata.
//
// Symbols are the core building blocks of the code index. They are stored in a hash map
// using FullyQualifiedName as the key for O(1) lookups.
type Symbol struct {
	Name               string     `json:"name"`
	FullyQualifiedName string     `json:"fqn"`
	Kind               SymbolKind `json:"kind"`
	Location           Location   `json:"location"`

	// Metadata extracted via AST traversal
	Scope          string   `json:"scope"`           // "public", "private", "protected"
	Modifiers      []string `json:"modifiers"`       // ["static", "async", "readonly", "abstract", "const", "unsafe"]
	Parameters     []string `json:"parameters"`      // Parameter names
	ParameterTypes []string `json:"parameter_types"` // Parameter types (if available)
	ReturnType     string   `json:"return_type"`     // Return type (if available)
	IsExported     bool     `json:"is_exported"`     // Whether exported from module
}

// SymbolKind identifies the type of symbol.
//
// Covers all major declaration types across supported languages.
type SymbolKind string

const (
	SymbolKindFunction  SymbolKind = "function"
	SymbolKindClass     SymbolKind = "class"
	SymbolKindInterface SymbolKind = "interface"
	SymbolKindType      SymbolKind = "type"
	SymbolKindVariable  SymbolKind = "variable"
	SymbolKindConstant  SymbolKind = "constant"
	SymbolKindEnum      SymbolKind = "enum"
	SymbolKindMethod    SymbolKind = "method"
	SymbolKindProperty  SymbolKind = "property"
)

// ImportInfo represents an import statement.
//
// Tracks which symbols are imported from which modules, enabling cross-file
// symbol resolution in the import graph phase.
type ImportInfo struct {
	Source          string            // Module path (e.g., './utils' or 'lodash')
	ResolvedPath    string            // Absolute path (if local file)
	ImportedSymbols map[string]string // localName → exportedName
	IsExternal      bool              // true if external library, false if local file
	ImportType      ImportType        // named, default, namespace
	Namespace       string            // For namespace imports (import * as foo)
	Location        Location
}

// ImportType identifies the type of import statement.
type ImportType string

const (
	ImportTypeNamed     ImportType = "named"     // import { foo, bar } from './mod'
	ImportTypeDefault   ImportType = "default"   // import foo from './mod'
	ImportTypeNamespace ImportType = "namespace" // import * as foo from './mod'
)

// ExportInfo represents an export statement.
//
// Tracks which symbols are exported from a module, enabling import resolution
// when other files import from this module.
type ExportInfo struct {
	Name         string
	ExportType   ExportType
	Kind         string // Symbol kind (function, class, etc.)
	Source       string // For re-exports (export { foo } from './mod')
	ResolvedPath string // Absolute path (if re-export from local file)
	Location     Location
}

// ExportType identifies the type of export statement.
type ExportType string

const (
	ExportTypeNamed     ExportType = "named"     // export { foo }
	ExportTypeDefault   ExportType = "default"   // export default foo
	ExportTypeNamespace ExportType = "namespace" // export * from './mod'
	ExportTypeReExport  ExportType = "re-export" // export { foo } from './mod'
)

// Location represents a position in source code.
//
// Uses 1-based line/column numbers for LSP compatibility.
// Uses 0-based byte offsets from tree-sitter for O(1) code slicing.
//
// Line/column numbers are human-readable and used for error messages and LSP.
// Byte offsets enable fast code extraction: sourceCode[StartByte:EndByte]
type Location struct {
	FilePath    string `json:"file_path"`
	StartLine   uint32 `json:"start_line"`   // 1-based line number (LSP compatible)
	StartColumn uint32 `json:"start_column"` // 1-based column number (LSP compatible)
	EndLine     uint32 `json:"end_line"`
	EndColumn   uint32 `json:"end_column"`
	StartByte   uint32 `json:"start_byte"` // 0-indexed byte offset (inclusive) - for fast code slicing
	EndByte     uint32 `json:"end_byte"`   // 0-indexed byte offset (exclusive) - for fast code slicing
}

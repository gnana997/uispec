// Import and export extraction implementation.
package extractor

import (
	"path/filepath"
	"strings"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
)

// extractImports processes import query matches into ImportInfo structs.
//
// Handles all import styles:
// - TypeScript/JavaScript: ES6 imports, CommonJS require
// - Python: import and from...import statements
// - Go: import declarations
// - Rust: use statements
func (e *Extractor) extractImports(matches []queries.QueryMatch, sourceCode []byte, filePath string, lang parser.Language) []ImportInfo {
	imports := make([]ImportInfo, 0)

	for _, match := range matches {
		// Filter to import-related captures
		if !e.isImportMatch(match) {
			continue
		}

		importInfo := e.buildImportInfo(match, sourceCode, filePath, lang)
		if importInfo != nil {
			imports = append(imports, *importInfo)
		}
	}

	return imports
}

// extractExports processes export query matches into ExportInfo structs.
//
// Handles all export styles:
// - TypeScript/JavaScript: ES6 exports, CommonJS module.exports
// - Python: __all__ list (if present)
// - Go: exported symbols (uppercase first letter)
// - Rust: pub declarations
func (e *Extractor) extractExports(matches []queries.QueryMatch, sourceCode []byte, filePath string, lang parser.Language) []ExportInfo {
	exports := make([]ExportInfo, 0)

	for _, match := range matches {
		// Filter to export-related captures
		if !e.isExportMatch(match) {
			continue
		}

		// Check if this is a CommonJS export
		isCommonJS := false
		for _, capture := range match.Captures {
			if strings.Contains(capture.Field, "commonjs") || strings.Contains(capture.Category, "commonjs") {
				isCommonJS = true
				break
			}
		}

		var exportInfo *ExportInfo
		if isCommonJS {
			exportInfo = e.buildCommonJSExportInfo(match, sourceCode, filePath)
		} else {
			exportInfo = e.buildExportInfo(match, sourceCode, filePath, lang)
		}

		if exportInfo != nil {
			exports = append(exports, *exportInfo)
		}
	}

	return exports
}

// isImportMatch checks if a query match contains import-related captures.
func (e *Extractor) isImportMatch(match queries.QueryMatch) bool {
	for _, capture := range match.Captures {
		if strings.HasPrefix(capture.Category, "import") {
			return true
		}
	}
	return false
}

// isExportMatch checks if a query match contains export-related captures.
func (e *Extractor) isExportMatch(match queries.QueryMatch) bool {
	for _, capture := range match.Captures {
		if strings.HasPrefix(capture.Category, "export") {
			return true
		}
	}
	return false
}

// buildImportInfo creates ImportInfo from query captures.
func (e *Extractor) buildImportInfo(match queries.QueryMatch, sourceCode []byte, filePath string, lang parser.Language) *ImportInfo {
	// Extract source (module path)
	sourceCapture := e.findCapture(match.Captures, "import", "source")
	if sourceCapture == nil {
		// Try CommonJS source
		sourceCapture = e.findCapture(match.Captures, "import", "commonjs.source")
	}
	if sourceCapture == nil {
		// Try alternate field names
		sourceCapture = e.findCapture(match.Captures, "import", "module")
		if sourceCapture == nil {
			sourceCapture = e.findCapture(match.Captures, "import", "path")
		}
	}

	if sourceCapture == nil {
		return nil // No source = invalid import
	}

	source := strings.Trim(sourceCapture.Text, "\"'")

	// Determine if external (doesn't start with . or /)
	isExternal := !strings.HasPrefix(source, ".") && !strings.HasPrefix(source, "/")

	// Resolve path for local imports
	var resolvedPath string
	if !isExternal {
		resolvedPath = e.resolveImportPath(source, filePath, lang)
	}

	// Extract imported symbols
	importedSymbols := make(map[string]string)
	importType := ImportTypeNamed
	namespace := ""

	// Check for namespace import (import * as foo OR const foo = require(...))
	namespaceCapture := e.findCapture(match.Captures, "import", "namespace")
	if namespaceCapture == nil {
		// Try CommonJS namespace (simple require)
		namespaceCapture = e.findCapture(match.Captures, "import", "commonjs.namespace")
	}
	if namespaceCapture != nil {
		namespace = namespaceCapture.Text
		importedSymbols[namespace] = "*"
		importType = ImportTypeNamespace
	}

	// Check for default import (import foo)
	defaultCapture := e.findCapture(match.Captures, "import", "default")
	if defaultCapture != nil {
		defaultName := defaultCapture.Text
		importedSymbols[defaultName] = "default"
		importType = ImportTypeDefault
	}

	// Check for named imports (import { foo, bar } OR const { foo } = require(...))
	for _, capture := range match.Captures {
		if capture.Field == "named" || capture.Field == "name" || capture.Field == "commonjs.named" {
			localName := capture.Text
			exportedName := localName // Default: same name

			// Check for alias (import { foo as bar })
			aliasCapture := e.findCaptureAfter(match.Captures, "import", "alias", &capture)
			if aliasCapture != nil {
				exportedName = localName
				localName = aliasCapture.Text
			}

			importedSymbols[localName] = exportedName
			importType = ImportTypeNamed
		}
	}

	// Get location from first capture
	var location Location
	if len(match.Captures) > 0 {
		firstCapture := match.Captures[0]
		location = e.extractLocation(firstCapture.Node, filePath)
	}

	return &ImportInfo{
		Source:          source,
		ResolvedPath:    resolvedPath,
		ImportedSymbols: importedSymbols,
		IsExternal:      isExternal,
		ImportType:      importType,
		Namespace:       namespace,
		Location:        location,
	}
}

// buildExportInfo creates ExportInfo from query captures.
func (e *Extractor) buildExportInfo(match queries.QueryMatch, sourceCode []byte, filePath string, lang parser.Language) *ExportInfo {
	// Extract export name
	nameCapture := e.findCapture(match.Captures, "export", "name")
	if nameCapture == nil {
		// Try alternate field names
		nameCapture = e.findCapture(match.Captures, "export", "identifier")
		if nameCapture == nil {
			// Check for default export
			nameCapture = e.findCapture(match.Captures, "export", "default")
			if nameCapture != nil {
				// Default export
				location := e.extractLocation(nameCapture.Node, filePath)
				return &ExportInfo{
					Name:       "default",
					ExportType: ExportTypeDefault,
					Location:   location,
				}
			}
			return nil
		}
	}

	name := nameCapture.Text

	// Determine export type
	exportType := ExportTypeNamed

	// Check for re-export (export { foo } from './mod')
	sourceCapture := e.findCapture(match.Captures, "export", "source")
	var source string
	var resolvedPath string
	if sourceCapture != nil {
		source = strings.Trim(sourceCapture.Text, "\"'")
		resolvedPath = e.resolveImportPath(source, filePath, lang)
		exportType = ExportTypeReExport
	}

	// Get location
	location := e.extractLocation(nameCapture.Node, filePath)

	// Try to infer kind from node type
	kind := ""
	nodeType := nameCapture.Node.GrammarName()
	if strings.Contains(nodeType, "function") {
		kind = "function"
	} else if strings.Contains(nodeType, "class") {
		kind = "class"
	} else if strings.Contains(nodeType, "variable") {
		kind = "variable"
	}

	return &ExportInfo{
		Name:         name,
		ExportType:   exportType,
		Kind:         kind,
		Source:       source,
		ResolvedPath: resolvedPath,
		Location:     location,
	}
}

// buildCommonJSExportInfo creates ExportInfo from CommonJS module.exports captures.
//
// Handles all CommonJS export patterns:
// - module.exports = value → default export
// - module.exports = { foo, bar } → named exports
// - exports.foo = value → named export
// - module.exports.bar = value → named export
func (e *Extractor) buildCommonJSExportInfo(match queries.QueryMatch, sourceCode []byte, filePath string) *ExportInfo {
	// Check for default export: module.exports = value
	defaultCapture := e.findCapture(match.Captures, "export", "commonjs.default")
	if defaultCapture != nil {
		location := e.extractLocation(defaultCapture.Node, filePath)
		return &ExportInfo{
			Name:       "default",
			ExportType: ExportTypeDefault,
			Location:   location,
		}
	}

	// Named exports: exports.foo, module.exports.foo, module.exports = { foo }
	nameCapture := e.findCapture(match.Captures, "export", "commonjs.name")
	if nameCapture == nil {
		// Try alternate field names
		nameCapture = e.findCapture(match.Captures, "export.commonjs", "name")
	}

	if nameCapture != nil {
		name := nameCapture.Text
		location := e.extractLocation(nameCapture.Node, filePath)

		return &ExportInfo{
			Name:       name,
			ExportType: ExportTypeNamed,
			Location:   location,
		}
	}

	return nil
}


// resolveImportPath converts relative import path to absolute path.
//
// Examples:
// - './utils' → '/path/to/project/src/utils.ts'
// - '../lib' → '/path/to/project/lib/index.js'
//
// This is a simple resolution. Full module resolution (node_modules, etc.)
// will be handled in the ImportResolver phase.
func (e *Extractor) resolveImportPath(source string, currentFile string, lang parser.Language) string {
	// Get directory of current file
	dir := filepath.Dir(currentFile)

	// Resolve relative path
	absPath := filepath.Join(dir, source)
	absPath = filepath.Clean(absPath)

	// Try adding file extensions if not present
	if !hasExtension(absPath) {
		// Try common extensions for the language
		extensions := getLanguageExtensions(lang)
		for _, ext := range extensions {
			candidate := absPath + ext
			// In a real implementation, we'd check if file exists
			// For now, just use first extension
			return candidate
		}
	}

	return absPath
}

// hasExtension checks if a path has a file extension.
func hasExtension(path string) bool {
	return filepath.Ext(path) != ""
}

// getLanguageExtensions returns common file extensions for a language.
func getLanguageExtensions(lang parser.Language) []string {
	switch lang {
	case parser.LanguageTypeScript:
		return []string{".ts", ".tsx", ".d.ts"}
	case parser.LanguageJavaScript:
		return []string{".js", ".jsx", ".mjs", ".cjs"}
	default:
		return []string{}
	}
}

// findCapture finds a capture with matching category and field.
func (e *Extractor) findCapture(captures []queries.QueryCapture, category string, field string) *queries.QueryCapture {
	for i := range captures {
		if captures[i].Category == category && captures[i].Field == field {
			return &captures[i]
		}
	}
	return nil
}

// findCaptureAfter finds a capture with matching category and field that comes after the given capture.
//
// Used to find aliases that follow named imports.
func (e *Extractor) findCaptureAfter(captures []queries.QueryCapture, category string, field string, after *queries.QueryCapture) *queries.QueryCapture {
	afterFound := false
	for i := range captures {
		if &captures[i] == after {
			afterFound = true
			continue
		}
		if afterFound && captures[i].Category == category && captures[i].Field == field {
			return &captures[i]
		}
	}
	return nil
}


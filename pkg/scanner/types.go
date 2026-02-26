// Package scanner analyzes React component source files and detects components,
// their props, and compound component relationships.
package scanner

import (
	"github.com/gnana997/uispec/pkg/extractor"
)

// ScanConfig configures the scan command behavior.
type ScanConfig struct {
	// Include glob patterns for file matching.
	Include []string
	// Exclude glob patterns.
	Exclude []string
	// Framework hint (currently only "react" is supported).
	Framework string
}

// DefaultScanConfig returns the default scan configuration with
// scan-specific exclusions for test, story, and mock files.
func DefaultScanConfig() ScanConfig {
	return ScanConfig{
		Include: []string{
			"**/*.ts",
			"**/*.tsx",
			"**/*.js",
			"**/*.jsx",
		},
		Exclude: []string{
			"node_modules/**",
			".git/**",
			"dist/**",
			"build/**",
			".next/**",
			"coverage/**",
			"out/**",
			".vscode/**",
			".uispec/**",
			// Scan-specific: skip test/story/mock files
			"**/*.test.*",
			"**/*.spec.*",
			"**/*.stories.*",
			"**/*.story.*",
			"__tests__/**",
			"**/__tests__/**",
			"**/__mocks__/**",
			"**/__snapshots__/**",
		},
		Framework: "react",
	}
}

// FileExtractionResult holds extraction output plus source bytes
// needed for Phase 3 AST re-parsing.
type FileExtractionResult struct {
	FilePath   string
	SourceCode []byte
	Result     *extractor.PerFileResult
}

// ComponentKind describes how a component was declared.
type ComponentKind string

const (
	ComponentKindFunction   ComponentKind = "function"
	ComponentKindForwardRef ComponentKind = "forwardRef"
	ComponentKindMemo       ComponentKind = "memo"
	ComponentKindClass      ComponentKind = "class"
)

// PropsRef captures how the component's props type is referenced.
type PropsRef struct {
	// TypeName is the name of the props interface/type (e.g., "ButtonProps").
	TypeName string
	// IsInline is true if props are defined inline (no separate interface).
	IsInline bool
	// Symbol points to the matching interface/type Symbol in the same file.
	Symbol *extractor.Symbol
}

// DetectedComponent represents a React component found during Phase 3.
type DetectedComponent struct {
	Name            string
	FilePath        string
	Kind            ComponentKind
	IsExported      bool
	IsDefaultExport bool
	PropsRef        *PropsRef
	Symbol          *extractor.Symbol
}

// CompoundGroup represents components from the same file that form
// a compound component (e.g., Dialog + DialogTrigger + DialogContent).
type CompoundGroup struct {
	Parent        *DetectedComponent
	SubComponents []*DetectedComponent
}

// ScanResult is the output of Phases 1-3.
type ScanResult struct {
	Components     []DetectedComponent
	CompoundGroups []CompoundGroup
	Stats          ScanStats
}

// ScanStats tracks scan performance metrics.
type ScanStats struct {
	FilesDiscovered    int
	FilesExtracted     int
	FilesFailed        int
	ComponentsDetected int
	CompoundGroups     int
	PropsExtracted     int
	DiscoveryTimeMs    int64
	ExtractionTimeMs   int64
	DetectionTimeMs    int64
	PropExtractionTimeMs int64
	CatalogBuildTimeMs int64
	TotalTimeMs        int64
}

// ExtractedProp holds a single prop extracted from an interface/type.
type ExtractedProp struct {
	Name          string
	Type          string   // simplified: "string", "boolean", "ReactNode", "function", etc.
	Required      bool
	Default       string
	Description   string
	AllowedValues []string // from string literal unions
	Deprecated    bool
}

// PropExtractionResult holds all props for one component.
type PropExtractionResult struct {
	ComponentName string
	FilePath      string
	Props         []ExtractedProp
}

// CatalogBuildConfig configures catalog generation.
type CatalogBuildConfig struct {
	Name         string // catalog name (--name or directory basename)
	ImportPrefix string // e.g., "@/components/ui"
	RootDir      string // scanned directory (for relative import paths)
}

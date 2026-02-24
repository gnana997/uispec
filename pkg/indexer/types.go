package indexer

import (
	"time"

	"github.com/gnana997/uispec/pkg/extractor"
)

// FileSymbols contains all extracted data for a single file.
//
// This is the unit of caching in the SymbolIndexer. Each file's
// symbols, imports, and exports are stored together for efficient
// retrieval and invalidation.
type FileSymbols struct {
	// FilePath is the absolute path to the file
	FilePath string

	// Symbols extracted from the file
	Symbols []*extractor.Symbol

	// Imports found in the file
	Imports []*extractor.ImportInfo

	// Exports found in the file
	Exports []*extractor.ExportInfo

	// TypeAnnotations maps variable/parameter names to their declared types
	// Example: "service" → "UserService", "count" → "number"
	// Used for TypeScript/JavaScript method call resolution
	TypeAnnotations map[string]string

	// Timestamp when the file was indexed (Unix milliseconds)
	Timestamp int64

	// ContentHash is SHA-256 hash of file content (for change detection)
	// Optional - may be empty if not computed
	ContentHash string

	// TokenCount is approximate number of tokens in file
	// Used for chunking decisions in later phases
	TokenCount int
}

// SymbolIndexerConfig configures the symbol indexer behavior.
type SymbolIndexerConfig struct {
	// MaxCachedFiles is the maximum number of files to keep in the LRU cache.
	// When the cache is full, least recently used files are evicted.
	// Default: 1000 files
	MaxCachedFiles int

	// Debug enables verbose logging
	Debug bool
}

// DefaultSymbolIndexerConfig returns the default configuration.
func DefaultSymbolIndexerConfig() SymbolIndexerConfig {
	return SymbolIndexerConfig{
		MaxCachedFiles: 1000,
		Debug:          false,
	}
}

// SymbolIndexerStats provides statistics about the indexer state.
//
// These metrics are useful for monitoring performance, debugging,
// and understanding cache effectiveness.
type SymbolIndexerStats struct {
	// IndexedFiles is the total number of files indexed (including evicted)
	IndexedFiles int

	// TotalSymbols is the count of symbols currently in the index
	TotalSymbols int

	// CachedFiles is the number of files currently in the LRU cache
	CachedFiles int

	// DirtyFiles is the number of files marked for recomputation
	DirtyFiles int

	// CacheHits is the number of successful cache lookups
	CacheHits int64

	// CacheMisses is the number of failed cache lookups
	CacheMisses int64

	// CacheHitRate is the percentage of cache hits (0.0 - 1.0)
	CacheHitRate float64

	// Evictions is the number of LRU evictions that have occurred
	Evictions int64

	// MemoryEstimateBytes is rough estimate of memory usage
	// Calculation: (symbols * 200) + (cached files * 500KB)
	MemoryEstimateBytes int64

	// AverageIndexTimeMs is the average time to index a file
	AverageIndexTimeMs float64
}

// ScanOptions configures workspace scanning behavior.
type ScanOptions struct {
	// Include patterns (glob syntax, e.g., "**/*.ts")
	// If empty, uses default language extensions
	Include []string

	// Exclude patterns (glob syntax, e.g., "node_modules/**")
	// These are added to default exclusions
	Exclude []string

	// RespectGitignore if true, respects .gitignore files
	// Default: true
	RespectGitignore bool

	// MaxDepth limits directory traversal depth
	// 0 = unlimited (default)
	MaxDepth int

	// FollowSymlinks if true, follows symbolic links
	// Default: false (avoid infinite loops)
	FollowSymlinks bool
}

// DefaultScanOptions returns recommended scan options.
func DefaultScanOptions() ScanOptions {
	return ScanOptions{
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
			".vscode/**",
			"coverage/**",
			"out/**",
			".next/**",
		},
		RespectGitignore: true,
		MaxDepth:         0,
		FollowSymlinks:   false,
	}
}

// ScanStats contains statistics about a workspace scan.
type ScanStats struct {
	// FilesDiscovered is the total number of files found
	FilesDiscovered int

	// FilesIndexed is the number of files successfully indexed
	FilesIndexed int

	// FilesFailed is the number of files that failed to index
	FilesFailed int

	// FilesSkipped is the number of files skipped (e.g., excluded)
	FilesSkipped int

	// SymbolsExtracted is the total number of symbols extracted
	SymbolsExtracted int

	// ImportsExtracted is the total number of imports extracted
	ImportsExtracted int

	// ExportsExtracted is the total number of exports extracted
	ExportsExtracted int

	// TotalTimeMs is the total scan duration in milliseconds
	TotalTimeMs int64

	// DiscoveryTimeMs is time spent discovering files
	DiscoveryTimeMs int64

	// IndexingTimeMs is time spent indexing files
	IndexingTimeMs int64

	// AverageFileTimeMs is average time per file
	AverageFileTimeMs float64

	// FilesPerSecond is the throughput rate
	FilesPerSecond float64

	// PeakMemoryBytes is the peak memory usage during scan
	PeakMemoryBytes int64

	// WorkerCount is the number of workers used
	WorkerCount int

	// SuccessRate is the percentage of files successfully indexed (0.0 - 1.0)
	SuccessRate float64

	// Errors contains per-file errors (if any)
	Errors []FileError

	// Cancelled indicates if the scan was cancelled
	Cancelled bool

	// StartTime is when the scan started
	StartTime time.Time

	// EndTime is when the scan completed
	EndTime time.Time
}

// FileError represents an error that occurred while processing a file.
type FileError struct {
	FilePath string
	Error    error
}

// ProgressCallback is called periodically during workspace scanning.
//
// Parameters:
//   - indexed: Number of files indexed so far
//   - total: Total number of files to index
//   - currentFile: Path of the file currently being indexed
type ProgressCallback func(indexed, total int, currentFile string)

// WatchOptions configures file watching behavior.
type WatchOptions struct {
	// DebounceMs is the debounce delay in milliseconds
	// Multiple rapid changes are grouped into a single reindex
	// Default: 200ms
	DebounceMs int

	// IgnorePatterns are patterns to ignore during watching
	// Uses same syntax as ScanOptions.Exclude
	IgnorePatterns []string

	// BatchSize is the number of files to batch before reindexing
	// Default: 1 (immediate reindex)
	BatchSize int
}

// DefaultWatchOptions returns recommended watch options.
func DefaultWatchOptions() WatchOptions {
	return WatchOptions{
		DebounceMs: 200,
		IgnorePatterns: []string{
			"**/*.swp",
			"**/*.tmp",
			"**/*~",
			".git/**",
		},
		BatchSize: 1,
	}
}

// WatchEvent represents a file system change event.
type WatchEvent struct {
	// FilePath is the absolute path to the changed file
	FilePath string

	// Op is the operation that occurred (Create, Write, Remove, Rename, Chmod)
	Op string

	// Timestamp is when the event occurred
	Timestamp time.Time
}

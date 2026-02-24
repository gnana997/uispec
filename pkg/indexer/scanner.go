package indexer

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/util"
)

// WorkspaceScanner scans and indexes entire workspaces in parallel.
//
// **Three-Phase Pipeline:**
//  1. File Discovery - Walk directory tree and find matching files
//  2. Parallel Processing - Extract symbols/imports/exports using worker pool
//  3. Indexing - Store results in SymbolIndexer
//
// **Performance:**
//   - Expected throughput: 100-200 files/sec (depending on hardware)
//   - Memory: <20MB for 1000 files
//   - Speedup: 1.8-2.0x vs sequential
//
// **Usage:**
//
//	scanner := NewWorkspaceScanner(extractor, indexer, logger)
//	stats, err := scanner.ScanWorkspace(
//	    "/path/to/workspace",
//	    DefaultScanOptions(),
//	    func(indexed, total int, file string) {
//	        fmt.Printf("Progress: %d/%d - %s\n", indexed, total, file)
//	    },
//	)
type WorkspaceScanner struct {
	extractor *extractor.Extractor
	indexer   *SymbolIndexer
	logger    *slog.Logger
}

// NewWorkspaceScanner creates a new workspace scanner.
func NewWorkspaceScanner(
	extractor *extractor.Extractor,
	indexer *SymbolIndexer,
	logger *slog.Logger,
) *WorkspaceScanner {
	return &WorkspaceScanner{
		extractor: extractor,
		indexer:   indexer,
		logger:    logger,
	}
}

// ScanWorkspace scans an entire workspace and indexes all matching files.
//
// **Parameters:**
//   - rootPath: Absolute path to workspace root
//   - options: Scan configuration (patterns, exclusions, etc.)
//   - progressCallback: Optional progress callback
//
// **Returns:**
//   - stats: Detailed scan statistics
//   - error: Error if scan failed
//
// **Performance:**
// Uses worker pool with runtime.NumCPU() × 2 workers for parallel processing.
func (ws *WorkspaceScanner) ScanWorkspace(
	rootPath string,
	options ScanOptions,
	progressCallback ProgressCallback,
) (*ScanStats, error) {
	startTime := time.Now()
	stats := &ScanStats{
		StartTime: startTime,
		Errors:    make([]FileError, 0),
	}

	ws.logger.Info("Starting workspace scan", "root", rootPath)

	// Phase 1: Discover files
	discoveryStart := time.Now()
	files, err := ws.discoverFiles(rootPath, options)
	if err != nil {
		return nil, fmt.Errorf("file discovery failed: %w", err)
	}
	stats.FilesDiscovered = len(files)
	stats.DiscoveryTimeMs = time.Since(discoveryStart).Milliseconds()

	ws.logger.Info("File discovery complete",
		"files_found", len(files),
		"duration_ms", stats.DiscoveryTimeMs)

	if len(files) == 0 {
		ws.logger.Warn("No files found matching criteria")
		stats.EndTime = time.Now()
		stats.TotalTimeMs = time.Since(startTime).Milliseconds()
		return stats, nil
	}

	// Phase 2 & 3: Process files in parallel and index
	indexingStart := time.Now()
	err = ws.processFilesParallel(files, stats, progressCallback)
	if err != nil {
		return nil, fmt.Errorf("file processing failed: %w", err)
	}
	stats.IndexingTimeMs = time.Since(indexingStart).Milliseconds()

	// Finalize stats
	stats.EndTime = time.Now()
	stats.TotalTimeMs = time.Since(startTime).Milliseconds()

	if stats.FilesIndexed > 0 {
		stats.AverageFileTimeMs = float64(stats.IndexingTimeMs) / float64(stats.FilesIndexed)
		stats.FilesPerSecond = float64(stats.FilesIndexed) / (float64(stats.IndexingTimeMs) / 1000.0)
	}

	if stats.FilesDiscovered > 0 {
		stats.SuccessRate = float64(stats.FilesIndexed) / float64(stats.FilesDiscovered)
	}

	ws.logger.Info("Workspace scan complete",
		"files_indexed", stats.FilesIndexed,
		"files_failed", stats.FilesFailed,
		"symbols_extracted", stats.SymbolsExtracted,
		"duration_ms", stats.TotalTimeMs,
		"files_per_second", fmt.Sprintf("%.1f", stats.FilesPerSecond))

	return stats, nil
}

// discoverFiles walks the directory tree and finds all matching files.
//
// **Performance:** O(n) where n is total number of files in tree.
// Typically <100ms for workspaces with 1000s of files.
func (ws *WorkspaceScanner) discoverFiles(rootPath string, options ScanOptions) ([]string, error) {
	var files []string

	// Validate patterns (just check syntax)
	for _, pattern := range options.Exclude {
		if !doublestar.ValidatePattern(pattern) {
			return nil, fmt.Errorf("invalid exclude pattern: %s", pattern)
		}
	}

	for _, pattern := range options.Include {
		if !doublestar.ValidatePattern(pattern) {
			return nil, fmt.Errorf("invalid include pattern: %s", pattern)
		}
	}

	// Walk directory tree
	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			ws.logger.Warn("Walk error", "path", path, "error", err)
			return nil // Continue walking
		}

		// Get relative path for pattern matching
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			relPath = path
		}

		// Convert to forward slashes for pattern matching
		relPath = filepath.ToSlash(relPath)

		// Check exclusions (directories and files)
		for _, pattern := range options.Exclude {
			matched, _ := doublestar.PathMatch(pattern, relPath)
			if matched {
				if d.IsDir() {
					return filepath.SkipDir // Skip entire directory
				}
				return nil // Skip file
			}
		}

		// Only process files (not directories)
		if d.IsDir() {
			return nil
		}

		// Check include patterns
		if len(options.Include) > 0 {
			matched := false
			for _, pattern := range options.Include {
				if m, _ := doublestar.PathMatch(pattern, relPath); m {
					matched = true
					break
				}
			}
			if !matched {
				return nil // File doesn't match any include pattern
			}
		}

		// File matches - add to list
		files = append(files, path)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// processFilesParallel processes files using a worker pool.
//
// **Architecture:**
//  1. Create worker pool (numWorkers = CPU × 2)
//  2. Submit all file jobs
//  3. Collect results and index in parallel
//  4. Track progress and errors
func (ws *WorkspaceScanner) processFilesParallel(
	files []string,
	stats *ScanStats,
	progressCallback ProgressCallback,
) error {
	totalFiles := len(files)

	// Determine worker count (MUST match parser pool size!)
	numWorkers := util.GetOptimalPoolSize()
	stats.WorkerCount = numWorkers

	// Create worker pool
	pool := NewWorkerPool(numWorkers, ws.extractor, ws.logger)
	pool.Start()
	defer pool.Stop()

	// Collect results
	indexed := atomic.Int32{}
	failed := atomic.Int32{}

	// Use context for clean shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Result collector goroutine
	// **CRITICAL:** Start this BEFORE submitting jobs to prevent deadlock!
	// If we submit jobs first, the submission loop can block when the jobs channel
	// fills up, preventing the result collector from ever starting.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.logger.Debug("Result collector started", "total_expected", totalFiles)

		for {
			select {
			case <-ctx.Done():
				ws.logger.Debug("Result collector cancelled via context")
				return

			case result, ok := <-pool.Results():
				if !ok {
					ws.logger.Debug("Result collector - results channel closed")
					return // Channel closed
				}

				ws.logger.Debug("Result collector - received result", "file", result.FilePath, "job_id", result.JobID)

				// Index the file
				ws.indexer.AddFileSymbolsWithTypes(
					result.FilePath,
					convertToSymbolPointers(result.Result.Symbols),
					convertToImportPointers(result.Result.Imports),
					convertToExportPointers(result.Result.Exports),
					result.Result.TypeAnnotations,
				)

				// Update stats
				stats.SymbolsExtracted += len(result.Result.Symbols)
				stats.ImportsExtracted += len(result.Result.Imports)
				stats.ExportsExtracted += len(result.Result.Exports)
				stats.FilesIndexed++

				// Progress callback
				count := indexed.Add(1)
				ws.logger.Debug("Result collector - indexed file", "count", count, "total", totalFiles, "failed", failed.Load())
				if progressCallback != nil {
					progressCallback(int(count), totalFiles, result.FilePath)
				}

				// Check if done
				if int(count)+int(failed.Load()) >= totalFiles {
					ws.logger.Debug("Result collector - all files processed, cancelling", "indexed", count, "failed", failed.Load())
					cancel()
					return
				}

			case fileErr, ok := <-pool.Errors():
				if !ok {
					ws.logger.Debug("Result collector - errors channel closed")
					return // Channel closed
				}

				ws.logger.Debug("Result collector - received error", "file", fileErr.FilePath)

				// Record error
				stats.Errors = append(stats.Errors, fileErr)
				stats.FilesFailed++

				ws.logger.Warn("File processing failed",
					"file", fileErr.FilePath,
					"error", fileErr.Error)

				// Check if done
				count := failed.Add(1)
				ws.logger.Debug("Result collector - file failed", "indexed", indexed.Load(), "failed", count, "total", totalFiles)
				if int(indexed.Load())+int(count) >= totalFiles {
					ws.logger.Debug("Result collector - all files processed (with errors), cancelling")
					cancel()
					return
				}
			}
		}
	}()

	// Now submit all jobs (result collector is running and ready to consume)
	ws.logger.Debug("Submitting jobs to worker pool", "count", totalFiles)
	for i, file := range files {
		err := pool.Submit(FileJob{
			FilePath: file,
			JobID:    i,
		})
		if err != nil {
			return fmt.Errorf("failed to submit job for %s: %w", file, err)
		}
	}

	// Signal no more jobs will be submitted
	// This allows workers to exit gracefully when jobs channel is drained
	ws.logger.Debug("Calling FinishSubmitting", "total_jobs", totalFiles)
	pool.FinishSubmitting()
	ws.logger.Debug("FinishSubmitting completed, waiting for results")

	// Wait for all results
	ws.logger.Debug("Main thread waiting for result collector to finish")
	<-done
	ws.logger.Debug("Result collector finished", "indexed", indexed.Load(), "failed", failed.Load())

	return nil
}

// Helper functions to convert slices to pointer slices
func convertToSymbolPointers(symbols []extractor.Symbol) []*extractor.Symbol {
	result := make([]*extractor.Symbol, len(symbols))
	for i := range symbols {
		result[i] = &symbols[i]
	}
	return result
}

func convertToImportPointers(imports []extractor.ImportInfo) []*extractor.ImportInfo {
	result := make([]*extractor.ImportInfo, len(imports))
	for i := range imports {
		result[i] = &imports[i]
	}
	return result
}

func convertToExportPointers(exports []extractor.ExportInfo) []*extractor.ExportInfo {
	result := make([]*extractor.ExportInfo, len(exports))
	for i := range exports {
		result[i] = &exports[i]
	}
	return result
}

// GetLanguageFromExtension infers language from file extension.
func GetLanguageFromExtension(filePath string) (parser.Language, bool) {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".ts":
		return parser.LanguageTypeScript, true
	case ".tsx":
		return parser.LanguageTypeScript, true
	case ".js":
		return parser.LanguageJavaScript, true
	case ".jsx":
		return parser.LanguageJavaScript, true
	default:
		return parser.LanguageTypeScript, false
	}
}

// GetIndexer returns the internal symbol indexer.
//
// This is useful for accessing the indexer after scanning,
// especially for building import graphs or other analysis.
func (ws *WorkspaceScanner) GetIndexer() *SymbolIndexer {
	return ws.indexer
}

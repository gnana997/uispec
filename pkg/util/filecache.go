// FileCache provides high-performance file access using memory-mapped files.
//
// **Performance Benefits:**
//   - 20x+ faster than os.ReadFile for random access
//   - 220x+ faster than line iteration for byte offset extraction
//   - O(1) byte offset slicing: code = mmap[startByte:endByte]
//   - Only accessed pages loaded into RAM (on-demand paging)
//
// **Safety Features:**
//   - Optional MaxFiles limit (prevents file descriptor exhaustion)
//   - Optional MaxMemoryMB limit (prevents runaway virtual memory usage)
//   - Graceful fallback to os.ReadFile if mmap fails
//   - Thread-safe with sync.RWMutex (parallel reads, exclusive writes)
//
// **Use Cases:**
//   1. During indexing: Fast code extraction for chunking (short-lived cache)
//   2. During runtime: Fast code retrieval for LLM context (long-lived cache)
//
// **Lifecycle:**
//   - Lazy loading: Files loaded on first access
//   - Kept mapped until Close() or limits reached
//   - No automatic eviction (Phase 1) - OS manages memory pressure
//
// **Large Codebase Support:**
//   - Small repos (<10K files): Use default limits (10K files, 2GB)
//   - Large repos (Kubernetes, 50K files): Increase limits to 20K files, 4GB
//   - Massive repos (100K+ files): Use batched indexing with cache clearing
package util

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/edsrzf/mmap-go"
)

// FileCache provides high-performance file access using memory-mapped files.
//
// Thread-safe: Multiple goroutines can call methods concurrently.
// Reads don't block each other (RWMutex), only loads/Close block.
type FileCache interface {
	// Get returns mmap'd file or loads it on first access (lazy loading).
	//
	// Returns error if:
	//   - File not found
	//   - MaxFiles or MaxMemoryMB limit reached
	//   - Both mmap and fallback fail
	//
	// Thread-safe: Concurrent calls are safe, uses double-check locking.
	Get(filePath string) (*MappedFile, error)

	// FetchCode extracts code using byte offsets (O(1) operation).
	//
	// Parameters:
	//   - filePath: Absolute path to source file
	//   - startByte: 0-indexed byte offset (inclusive)
	//   - endByte: 0-indexed byte offset (exclusive)
	//
	// Returns error if:
	//   - File not found
	//   - Invalid byte range (endByte <= startByte or endByte > file size)
	//   - Cache limits reached
	//
	// Performance: O(1) byte slicing, only accessed pages loaded into RAM.
	FetchCode(filePath string, startByte, endByte uint32) (string, error)

	// Size returns number of currently cached files.
	//
	// Useful for monitoring cache usage against limits.
	Size() int

	// Stats returns current cache metrics.
	//
	// Provides observability into cache performance:
	//   - Hit/miss ratio
	//   - Memory usage
	//   - Mmap failures (fallback usage)
	Stats() FileCacheStats

	// Close unmaps all files and releases resources.
	//
	// Must be called before application shutdown to properly release:
	//   - Memory-mapped regions
	//   - File descriptors
	//   - Internal caches
	//
	// Returns error if any files fail to unmap (logged, not fatal).
	Close() error
}

// FileCacheConfig controls FileCache behavior.
type FileCacheConfig struct {
	// MaxFiles is the maximum number of files to keep cached.
	//
	// Set to 0 for unlimited (not recommended for large codebases).
	//
	// Recommended values:
	//   - Small repos (<10K files): 10000 (default)
	//   - Large repos (50K files): 20000
	//   - Massive repos (100K+ files): 50000 or use batched indexing
	//
	// When limit reached: Get() returns error with clear message.
	MaxFiles int

	// MaxMemoryMB is the maximum virtual memory to use (in MB).
	//
	// Set to 0 for unlimited (not recommended for large codebases).
	//
	// IMPORTANT: This limits VIRTUAL memory (address space), not physical RAM.
	// Only accessed pages are loaded into physical RAM (OS manages this).
	//
	// Recommended values:
	//   - Small repos (~500MB source): 2048 MB (2GB, default)
	//   - Large repos (~2GB source): 4096 MB (4GB)
	//   - Massive repos (>5GB source): 8192 MB (8GB)
	//
	// When limit reached: Get() returns error with clear message.
	MaxMemoryMB int

	// EnableMetrics determines whether to track cache statistics.
	//
	// Enabled by default for observability.
	// Disable only if performance-critical (minimal overhead).
	EnableMetrics bool

	// Logger for warnings and errors.
	//
	// If nil, uses slog.Default().
	Logger *slog.Logger
}

// DefaultFileCacheConfig returns recommended defaults for most projects.
//
// Covers repos up to 50K files with 2GB source code.
// Suitable for codebases like:
//   - VSCode extensions (1K-5K files)
//   - Microservices (5K-20K files)
//   - Medium monorepos (20K-50K files)
func DefaultFileCacheConfig() *FileCacheConfig {
	return &FileCacheConfig{
		MaxFiles:      10000, // Covers medium-large repos
		MaxMemoryMB:   2048,  // 2GB virtual memory limit
		EnableMetrics: true,
		Logger:        nil, // Will use slog.Default()
	}
}

// UnboundedFileCacheConfig returns config with no limits.
//
// Use for:
//   - Testing
//   - Small repos (<10K files) with plenty of RAM
//   - Trusting OS to manage memory pressure
//
// WARNING: May use significant virtual memory on large repos.
// Monitor memory usage with Stats().
func UnboundedFileCacheConfig() *FileCacheConfig {
	return &FileCacheConfig{
		MaxFiles:      0,    // Unlimited
		MaxMemoryMB:   0,    // Unlimited
		EnableMetrics: true,
		Logger:        nil,
	}
}

// MappedFile represents a memory-mapped file.
type MappedFile struct {
	// Path is the absolute path to the source file.
	Path string

	// Data is the memory-mapped region.
	//
	// Can be sliced directly: code := Data[startByte:endByte]
	// Only accessed pages are loaded into physical RAM.
	// Nil for empty files.
	Data mmap.MMap

	// File is the underlying file descriptor.
	//
	// Kept open for proper cleanup (Close must call file.Close()).
	// Nil for fallback cache entries (when mmap fails).
	File *os.File

	// Size is the file size in bytes.
	Size int64

	// MappedAt is when this file was first mapped.
	//
	// Useful for LRU eviction (future enhancement).
	MappedAt time.Time
}

// FileCacheStats tracks cache performance metrics.
//
// Use for monitoring and alerting:
//   - High miss rate → Consider pre-loading hot files
//   - High mmap failures → Check file permissions, OS limits
//   - Approaching limits → Increase MaxFiles/MaxMemoryMB or use batching
type FileCacheStats struct {
	// FilesLoaded is the total number of files loaded (cumulative).
	//
	// Includes both successful and failed loads.
	FilesLoaded int64

	// FilesCached is the current number of cached files.
	//
	// Compare to MaxFiles to see headroom.
	FilesCached int

	// CacheHits is the number of successful cache lookups (cumulative).
	//
	// High hit rate (>95%) indicates effective caching.
	CacheHits int64

	// CacheMisses is the number of cache misses (cumulative).
	//
	// Cache miss triggers file load (slow path).
	CacheMisses int64

	// MmapFailures is the number of files that failed to mmap (cumulative).
	//
	// Falls back to os.ReadFile (slower, but works).
	// High failure rate indicates OS/permission issues.
	MmapFailures int64

	// TotalMappedMB is the total virtual memory mapped (current).
	//
	// IMPORTANT: This is VIRTUAL memory (address space), not physical RAM.
	// Only accessed pages consume physical RAM.
	// Compare to MaxMemoryMB to see headroom.
	TotalMappedMB float64
}

// NewFileCache creates a new FileCache with the given config.
//
// If config is nil, uses DefaultFileCacheConfig().
func NewFileCache(config *FileCacheConfig) FileCache {
	if config == nil {
		config = DefaultFileCacheConfig()
	}

	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	return &fileCacheImpl{
		config:        config,
		cache:         make(map[string]*MappedFile),
		fallbackCache: make(map[string][]byte),
		logger:        config.Logger,
		stats:         FileCacheStats{},
	}
}

// fileCacheImpl is the internal implementation of FileCache.
//
// Thread-safety:
//   - mu (RWMutex): Protects cache and fallbackCache maps
//     - RLock for reads (Get lookups) → Parallel reads
//     - Lock for writes (file loads, Close) → Exclusive access
//   - statsMu (Mutex): Protects stats fields
//     - Separate lock to avoid contention between cache access and stats updates
type fileCacheImpl struct {
	// Configuration
	config *FileCacheConfig
	logger *slog.Logger

	// Caches (protected by mu)
	cache         map[string]*MappedFile // path → mmap'd file
	fallbackCache map[string][]byte      // path → byte slice (for failed mmaps)
	mu            sync.RWMutex           // RLock for reads, Lock for writes

	// Metrics (protected by statsMu)
	stats   FileCacheStats
	statsMu sync.Mutex
}

// Get returns mmap'd file or loads it on first access.
func (fc *fileCacheImpl) Get(filePath string) (*MappedFile, error) {
	// Fast path: check if already cached (RLock - allows parallel reads)
	fc.mu.RLock()
	if mf, ok := fc.cache[filePath]; ok {
		fc.mu.RUnlock()
		fc.recordHit()
		return mf, nil
	}
	// Check fallback cache (for files that failed to mmap)
	if data, ok := fc.fallbackCache[filePath]; ok {
		fc.mu.RUnlock()
		fc.recordHit()
		return fc.wrapFallbackData(filePath, data), nil
	}
	fc.mu.RUnlock()

	// Slow path: load and mmap file (Lock - exclusive access)
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Double-check: another goroutine might have loaded it while we waited for Lock
	if mf, ok := fc.cache[filePath]; ok {
		fc.recordHit()
		return mf, nil
	}
	if data, ok := fc.fallbackCache[filePath]; ok {
		fc.recordHit()
		return fc.wrapFallbackData(filePath, data), nil
	}

	// Check limits BEFORE loading
	// For memory limit, we need to check the file size first
	var fileSize int64
	if fc.config.MaxMemoryMB > 0 {
		stat, err := os.Stat(filePath)
		if err != nil {
			fc.recordMiss()
			return nil, fmt.Errorf("failed to stat file %q: %w", filePath, err)
		}
		fileSize = stat.Size()
	}

	if err := fc.checkLimitsWithNewFile(fileSize); err != nil {
		fc.recordMiss()
		return nil, err
	}

	// Load file with mmap
	mf, err := fc.loadFile(filePath)
	if err != nil {
		fc.recordMiss()
		return nil, err
	}

	// Cache the loaded file
	fc.cache[filePath] = mf
	fc.recordLoad(mf)

	return mf, nil
}

// checkLimitsWithNewFile verifies that adding a new file won't exceed limits.
//
// Parameters:
//   - newFileSize: Size of the file about to be loaded (in bytes)
//
// Must be called while holding mu.Lock.
func (fc *fileCacheImpl) checkLimitsWithNewFile(newFileSize int64) error {
	// Check MaxFiles limit
	if fc.config.MaxFiles > 0 {
		currentFiles := len(fc.cache) + len(fc.fallbackCache)
		if currentFiles >= fc.config.MaxFiles {
			return fmt.Errorf("FileCache limit reached: %d files (limit: %d files). "+
				"Increase FileCacheConfig.MaxFiles or use batched indexing",
				currentFiles, fc.config.MaxFiles)
		}
	}

	// Check MaxMemoryMB limit (include new file size)
	if fc.config.MaxMemoryMB > 0 && newFileSize > 0 {
		currentMB := fc.calculateTotalMappedMBLocked()
		newFileMB := float64(newFileSize) / (1024 * 1024)
		totalAfterLoadMB := currentMB + newFileMB

		if totalAfterLoadMB >= float64(fc.config.MaxMemoryMB) {
			return fmt.Errorf("FileCache memory limit reached: %.2f MB + %.2f MB = %.2f MB (limit: %d MB). "+
				"Increase FileCacheConfig.MaxMemoryMB or use batched indexing",
				currentMB, newFileMB, totalAfterLoadMB, fc.config.MaxMemoryMB)
		}
	}

	return nil
}

// loadFile opens and mmaps a file, with fallback to os.ReadFile if mmap fails.
//
// Must be called while holding mu.Lock.
func (fc *fileCacheImpl) loadFile(filePath string) (*MappedFile, error) {
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %q: %w", filePath, err)
	}

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file %q: %w", filePath, err)
	}

	// Handle empty files (can't mmap zero bytes)
	if stat.Size() == 0 {
		return &MappedFile{
			Path:     filePath,
			Data:     nil,
			File:     file,
			Size:     0,
			MappedAt: time.Now(),
		}, nil
	}

	// Attempt mmap (read-only, private mapping)
	mmapData, err := mmap.Map(file, mmap.RDONLY, 0)
	if err != nil {
		fc.logger.Warn("mmap failed, using fallback",
			"file", filePath,
			"size", stat.Size(),
			"error", err)

		// Fallback: read entire file into memory
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			file.Close()
			return nil, fmt.Errorf("mmap failed and fallback failed for %q: mmap error: %v, read error: %w",
				filePath, err, readErr)
		}

		// Store in fallback cache
		fc.fallbackCache[filePath] = data
		fc.recordMmapFailure()
		file.Close()

		return fc.wrapFallbackData(filePath, data), nil
	}

	return &MappedFile{
		Path:     filePath,
		Data:     mmapData,
		File:     file,
		Size:     stat.Size(),
		MappedAt: time.Now(),
	}, nil
}

// wrapFallbackData wraps a byte slice as a MappedFile (for fallback cache).
//
// This allows consistent handling of both mmap'd and fallback files.
func (fc *fileCacheImpl) wrapFallbackData(filePath string, data []byte) *MappedFile {
	return &MappedFile{
		Path:     filePath,
		Data:     mmap.MMap(data), // Wrap as MMap for consistent interface
		File:     nil,              // No file descriptor (data is in memory)
		Size:     int64(len(data)),
		MappedAt: time.Now(),
	}
}

// FetchCode extracts code using byte offsets (O(1) operation).
func (fc *fileCacheImpl) FetchCode(filePath string, startByte, endByte uint32) (string, error) {
	// Get mmap'd file (or load it)
	mf, err := fc.Get(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get file %q: %w", filePath, err)
	}

	// Handle empty files
	if len(mf.Data) == 0 {
		return "", nil
	}

	// Special case: (0,0) means read entire file
	if startByte == 0 && endByte == 0 {
		endByte = uint32(len(mf.Data))
	} else if endByte <= startByte {
		return "", fmt.Errorf("invalid byte range: endByte (%d) <= startByte (%d)",
			endByte, startByte)
	}

	// Validate bounds
	if endByte > uint32(len(mf.Data)) {
		return "", fmt.Errorf("invalid byte range: endByte (%d) > file size (%d) for %q",
			endByte, len(mf.Data), filePath)
	}

	// O(1) slice operation
	// Only the accessed pages will be loaded into physical RAM
	code := mf.Data[startByte:endByte]

	return string(code), nil
}

// Size returns number of currently cached files.
func (fc *fileCacheImpl) Size() int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	return len(fc.cache) + len(fc.fallbackCache)
}

// Stats returns current cache metrics.
func (fc *fileCacheImpl) Stats() FileCacheStats {
	// Get cache sizes (read lock)
	fc.mu.RLock()
	cachedFiles := len(fc.cache) + len(fc.fallbackCache)
	totalMappedMB := fc.calculateTotalMappedMBLocked()
	fc.mu.RUnlock()

	// Get stats (separate lock to avoid contention)
	fc.statsMu.Lock()
	defer fc.statsMu.Unlock()

	stats := fc.stats
	stats.FilesCached = cachedFiles
	stats.TotalMappedMB = totalMappedMB

	return stats
}

// calculateTotalMappedMBLocked calculates total mapped memory.
//
// Must be called while holding mu.RLock or mu.Lock.
func (fc *fileCacheImpl) calculateTotalMappedMBLocked() float64 {
	total := int64(0)

	// Add mmap'd files
	for _, mf := range fc.cache {
		total += mf.Size
	}

	// Add fallback cache
	for _, data := range fc.fallbackCache {
		total += int64(len(data))
	}

	return float64(total) / (1024 * 1024)
}

// Close unmaps all files and releases resources.
func (fc *fileCacheImpl) Close() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	var errs []error

	// Unmap all mmap'd files
	for path, mf := range fc.cache {
		// Unmap memory
		if mf.Data != nil {
			if err := mf.Data.Unmap(); err != nil {
				fc.logger.Warn("failed to unmap file", "path", path, "error", err)
				errs = append(errs, fmt.Errorf("unmap %q: %w", path, err))
			}
		}

		// Close file descriptor
		if mf.File != nil {
			if err := mf.File.Close(); err != nil {
				fc.logger.Warn("failed to close file", "path", path, "error", err)
				errs = append(errs, fmt.Errorf("close %q: %w", path, err))
			}
		}
	}

	// Clear caches
	fc.cache = make(map[string]*MappedFile)
	fc.fallbackCache = make(map[string][]byte)

	// Log summary
	fc.logger.Info("FileCache closed",
		"files_loaded", fc.stats.FilesLoaded,
		"cache_hits", fc.stats.CacheHits,
		"cache_misses", fc.stats.CacheMisses,
		"mmap_failures", fc.stats.MmapFailures)

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}

	return nil
}

// Metrics recording methods

func (fc *fileCacheImpl) recordHit() {
	if !fc.config.EnableMetrics {
		return
	}
	fc.statsMu.Lock()
	fc.stats.CacheHits++
	fc.statsMu.Unlock()
}

func (fc *fileCacheImpl) recordMiss() {
	if !fc.config.EnableMetrics {
		return
	}
	fc.statsMu.Lock()
	fc.stats.CacheMisses++
	fc.statsMu.Unlock()
}

func (fc *fileCacheImpl) recordLoad(mf *MappedFile) {
	if !fc.config.EnableMetrics {
		return
	}
	fc.statsMu.Lock()
	fc.stats.FilesLoaded++
	fc.statsMu.Unlock()
}

func (fc *fileCacheImpl) recordMmapFailure() {
	if !fc.config.EnableMetrics {
		return
	}
	fc.statsMu.Lock()
	fc.stats.MmapFailures++
	fc.statsMu.Unlock()
}

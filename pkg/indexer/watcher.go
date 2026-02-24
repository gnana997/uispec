package indexer

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/gnana997/uispec/pkg/extractor"
)

// FileWatcher watches for file system changes and re-indexes files incrementally.
//
// **Features:**
//   - Debouncing - Groups rapid changes to avoid redundant reindexing
//   - Selective - Only reindexes changed files (not entire workspace)
//   - Fast - Typical reindex time: ~1-2ms per file
//
// **Usage:**
//
//	watcher := NewFileWatcher(scanner, indexer, extractor, DefaultWatchOptions(), logger)
//	err := watcher.Start("/path/to/workspace")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer watcher.Stop()
//
//	// Watcher runs in background, auto-reindexing changed files
type FileWatcher struct {
	watcher  *fsnotify.Watcher
	scanner  *WorkspaceScanner
	indexer  *SymbolIndexer
	extractor *extractor.Extractor
	logger   *slog.Logger
	options  WatchOptions

	// Debouncing
	debounceTimers map[string]*time.Timer
	debounceMu     sync.Mutex

	// Lifecycle
	stopChan chan struct{}
	stopped  bool
	mu       sync.Mutex
}

// NewFileWatcher creates a new file watcher.
func NewFileWatcher(
	scanner *WorkspaceScanner,
	indexer *SymbolIndexer,
	extractor *extractor.Extractor,
	options WatchOptions,
	logger *slog.Logger,
) *FileWatcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(fmt.Sprintf("failed to create file watcher: %v", err))
	}

	if options.DebounceMs == 0 {
		options.DebounceMs = 200 // Default debounce
	}

	return &FileWatcher{
		watcher:        watcher,
		scanner:        scanner,
		indexer:        indexer,
		extractor:      extractor,
		logger:         logger,
		options:        options,
		debounceTimers: make(map[string]*time.Timer),
		stopChan:       make(chan struct{}),
	}
}

// Start begins watching the specified directory.
//
// **Thread Safety:** Safe to call once. Panics if called multiple times.
//
// **Performance:** Runs in background goroutine.
func (fw *FileWatcher) Start(rootPath string) error {
	fw.mu.Lock()
	if fw.stopped {
		fw.mu.Unlock()
		return fmt.Errorf("watcher already stopped")
	}
	fw.mu.Unlock()

	// Add root directory to watch
	err := fw.watcher.Add(rootPath)
	if err != nil {
		return fmt.Errorf("failed to watch %s: %w", rootPath, err)
	}

	// Walk directory tree and add subdirectories
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on error
		}

		if info.IsDir() {
			// Check if directory should be ignored
			if fw.shouldIgnore(path) {
				return filepath.SkipDir
			}

			// Add directory to watcher
			if err := fw.watcher.Add(path); err != nil {
				fw.logger.Warn("Failed to watch directory", "path", path, "error", err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to setup watches: %w", err)
	}

	fw.logger.Info("File watcher started", "root", rootPath)

	// Start event loop
	go fw.eventLoop()

	return nil
}

// Stop stops the file watcher.
//
// **Thread Safety:** Safe to call multiple times (idempotent).
func (fw *FileWatcher) Stop() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.stopped {
		return nil
	}

	fw.stopped = true
	close(fw.stopChan)

	// Cancel all debounce timers
	fw.debounceMu.Lock()
	for _, timer := range fw.debounceTimers {
		timer.Stop()
	}
	fw.debounceTimers = make(map[string]*time.Timer)
	fw.debounceMu.Unlock()

	err := fw.watcher.Close()
	fw.logger.Info("File watcher stopped")
	return err
}

// eventLoop is the main event processing loop.
func (fw *FileWatcher) eventLoop() {
	for {
		select {
		case <-fw.stopChan:
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.logger.Error("File watcher error", "error", err)
		}
	}
}

// handleEvent processes a file system event.
func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	filePath := event.Name

	// Ignore if file should be ignored
	if fw.shouldIgnore(filePath) {
		return
	}

	// Only process source files
	if _, ok := GetLanguageFromExtension(filePath); !ok {
		return
	}

	fw.logger.Debug("File event", "op", event.Op.String(), "file", filePath)

	// Handle different event types
	switch {
	case event.Op&fsnotify.Write == fsnotify.Write:
		fw.debounceReindex(filePath)

	case event.Op&fsnotify.Create == fsnotify.Create:
		fw.debounceReindex(filePath)

	case event.Op&fsnotify.Remove == fsnotify.Remove:
		fw.removeFile(filePath)

	case event.Op&fsnotify.Rename == fsnotify.Rename:
		fw.removeFile(filePath)
	}
}

// debounceReindex schedules a reindex after debounce delay.
//
// If multiple events for the same file occur within debounce window,
// only the last one triggers reindexing (saves unnecessary work).
func (fw *FileWatcher) debounceReindex(filePath string) {
	fw.debounceMu.Lock()
	defer fw.debounceMu.Unlock()

	// Cancel existing timer if any
	if timer, exists := fw.debounceTimers[filePath]; exists {
		timer.Stop()
	}

	// Schedule new timer
	fw.debounceTimers[filePath] = time.AfterFunc(
		time.Duration(fw.options.DebounceMs)*time.Millisecond,
		func() {
			fw.reindexFile(filePath)

			// Clean up timer
			fw.debounceMu.Lock()
			delete(fw.debounceTimers, filePath)
			fw.debounceMu.Unlock()
		},
	)
}

// reindexFile re-indexes a single file.
func (fw *FileWatcher) reindexFile(filePath string) {
	fw.logger.Debug("Reindexing file", "file", filePath)

	// Mark as dirty first (instant feedback)
	fw.indexer.InvalidateFile(filePath)

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		fw.logger.Warn("Failed to read file for reindexing",
			"file", filePath,
			"error", err)
		return
	}

	// Extract symbols/imports/exports
	result, err := fw.extractor.ExtractFile(filePath, content)
	if err != nil {
		fw.logger.Warn("Failed to extract file",
			"file", filePath,
			"error", err)
		return
	}

	// Index the file
	fw.indexer.AddFileSymbolsWithTypes(
		filePath,
		convertToSymbolPointers(result.Symbols),
		convertToImportPointers(result.Imports),
		convertToExportPointers(result.Exports),
		result.TypeAnnotations,
	)

	fw.logger.Debug("File reindexed",
		"file", filePath,
		"symbols", len(result.Symbols),
		"imports", len(result.Imports),
		"exports", len(result.Exports))
}

// removeFile removes a file from the index.
func (fw *FileWatcher) removeFile(filePath string) {
	fw.logger.Debug("Removing file from index", "file", filePath)
	fw.indexer.RemoveFile(filePath)
}

// shouldIgnore checks if a path should be ignored.
func (fw *FileWatcher) shouldIgnore(path string) bool {
	// Check against ignore patterns
	for _, pattern := range fw.options.IgnorePatterns {
		// Simple substring match for now
		// In production, use glob matching
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}

	// Ignore common build/dependency directories
	base := filepath.Base(path)
	switch base {
	case "node_modules", ".git", "dist", "build", ".next":
		return true
	}

	return false
}

// GetStats returns file watcher statistics.
func (fw *FileWatcher) GetStats() FileWatcherStats {
	fw.debounceMu.Lock()
	pendingReindexes := len(fw.debounceTimers)
	fw.debounceMu.Unlock()

	return FileWatcherStats{
		PendingReindexes: pendingReindexes,
		IsRunning:        !fw.stopped,
	}
}

// FileWatcherStats contains file watcher statistics.
type FileWatcherStats struct {
	PendingReindexes int
	IsRunning        bool
}

package indexer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
	"github.com/gnana997/uispec/pkg/util"
)

// TestWorkerPool_Basic verifies basic worker pool functionality
func TestWorkerPool_Basic(t *testing.T) {
	logger := util.NewLogger(util.DefaultLoggerConfig())
	parserMgr := parser.NewParserManager(logger)
	defer parserMgr.Close()

	queryMgr := queries.NewQueryManager(parserMgr, logger)
	defer queryMgr.Close()

	ext := extractor.NewExtractor(parserMgr, queryMgr, logger)

	// Create worker pool
	pool := NewWorkerPool(4, ext, logger)
	pool.Start()
	defer pool.Stop()

	// Submit test jobs
	testFiles := []string{
		"test1.ts",
		"test2.ts",
		"test3.ts",
	}

	// Note: These files don't exist, so they'll error
	// This tests error handling
	for i, file := range testFiles {
		err := pool.Submit(FileJob{FilePath: file, JobID: i})
		assert.NoError(t, err)
	}

	// Collect results/errors
	errorCount := 0
	for i := 0; i < len(testFiles); i++ {
		select {
		case <-pool.Results():
			t.Fail() // Shouldn't get results for non-existent files
		case <-pool.Errors():
			errorCount++
		}
	}

	assert.Equal(t, len(testFiles), errorCount)
	stats := pool.GetStats()
	assert.Equal(t, int64(3), stats.JobsSubmitted)
	assert.Equal(t, int64(3), stats.JobsFailed)
}

// TestFileWatcher_Basic tests basic file watcher functionality
func TestFileWatcher_Basic(t *testing.T) {
	t.Skip("File watcher test requires manual file modifications - skipping in automated tests")

	logger := util.NewLogger(util.DefaultLoggerConfig())
	parserMgr := parser.NewParserManager(logger)
	defer parserMgr.Close()

	queryMgr := queries.NewQueryManager(parserMgr, logger)
	defer queryMgr.Close()

	ext := extractor.NewExtractor(parserMgr, queryMgr, logger)
	indexer := NewSymbolIndexer(DefaultSymbolIndexerConfig(), logger)
	defer indexer.Close()

	scanner := NewWorkspaceScanner(ext, indexer, logger)

	watcher := NewFileWatcher(scanner, indexer, ext, DefaultWatchOptions(), logger)

	// Create temp directory
	tempDir := t.TempDir()

	err := watcher.Start(tempDir)
	require.NoError(t, err)
	defer watcher.Stop()

	// Watcher should be running
	stats := watcher.GetStats()
	assert.True(t, stats.IsRunning)
}

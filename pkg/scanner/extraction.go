package scanner

import (
	"log/slog"
	"os"
	"sync"

	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/util"
)

// ExtractAll runs extractor.ExtractFile on each file in parallel.
// Returns FileExtractionResult with source bytes preserved for Phase 3.
// Errors on individual files are logged but don't stop the pipeline.
func ExtractAll(
	files []string,
	ext *extractor.Extractor,
	logger *slog.Logger,
) ([]FileExtractionResult, int) {
	if len(files) == 0 {
		return nil, 0
	}
	if logger == nil {
		logger = slog.Default()
	}

	numWorkers := util.GetOptimalPoolSize()
	if numWorkers > len(files) {
		numWorkers = len(files)
	}

	paths := make(chan string, numWorkers*2)
	type resultOrError struct {
		result FileExtractionResult
		err    error
		file   string
	}
	results := make(chan resultOrError, numWorkers)

	// Start workers.
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range paths {
				source, err := os.ReadFile(path)
				if err != nil {
					results <- resultOrError{err: err, file: path}
					continue
				}
				pfr, err := ext.ExtractFile(path, source)
				if err != nil {
					results <- resultOrError{err: err, file: path}
					continue
				}
				results <- resultOrError{
					result: FileExtractionResult{
						FilePath:   path,
						SourceCode: source,
						Result:     pfr,
					},
				}
			}
		}()
	}

	// Submit jobs.
	go func() {
		for _, f := range files {
			paths <- f
		}
		close(paths)
		wg.Wait()
		close(results)
	}()

	// Collect results.
	var extracted []FileExtractionResult
	failed := 0
	for r := range results {
		if r.err != nil {
			logger.Warn("extraction failed", "file", r.file, "error", r.err)
			failed++
			continue
		}
		extracted = append(extracted, r.result)
	}

	return extracted, failed
}

package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/util"
)

// FileJob represents a file to be processed by the worker pool.
type FileJob struct {
	FilePath string
	JobID    int
}

// FileResult contains the extraction result for a file.
type FileResult struct {
	FilePath string
	Result   *extractor.PerFileResult
	JobID    int
}

// WorkerPool manages a pool of goroutines for parallel file processing.
//
// **Architecture:**
//   - Goroutine-based (much lighter than OS threads)
//   - Buffered channels for job distribution
//   - Separate result and error channels
//   - Graceful shutdown support
//
// **Performance:**
//   - Worker count: runtime.NumCPU() × 2 (default)
//   - Expected speedup: 1.8-2.0x on typical hardware
//   - Memory: ~10MB for pool + worker overhead
//
// **Usage:**
//
//	pool := NewWorkerPool(numWorkers, extractor, logger)
//	pool.Start()
//	defer pool.Stop()
//
//	// Submit jobs
//	for _, file := range files {
//	    pool.Submit(FileJob{FilePath: file})
//	}
//
//	// Collect results
//	for i := 0; i < len(files); i++ {
//	    select {
//	    case result := <-pool.Results():
//	        // Process result
//	    case err := <-pool.Errors():
//	        // Handle error
//	    }
//	}
//
//	pool.Wait()
type WorkerPool struct {
	numWorkers int
	jobs       chan FileJob
	results    chan FileResult
	errors     chan FileError
	wg         sync.WaitGroup
	extractor  *extractor.Extractor
	logger     *slog.Logger

	// Lifecycle management
	ctx        context.Context
	cancel     context.CancelFunc
	started    atomic.Bool
	stopped    atomic.Bool
	jobsClosed atomic.Bool // Tracks if jobs channel has been closed

	// Statistics
	jobsSubmitted  atomic.Int64
	jobsProcessed  atomic.Int64
	jobsFailed     atomic.Int64
}

// NewWorkerPool creates a new worker pool.
//
// Parameters:
//   - numWorkers: Number of worker goroutines (0 = auto-detect)
//   - extractor: Extractor instance for processing files
//   - logger: Logger for worker messages
//
// Auto-detection uses: util.GetOptimalPoolSize() (matches parser pool size)
//
// **CRITICAL:** Worker count MUST match parser pool size to prevent blocking!
// Using util.GetOptimalPoolSize() ensures synchronization.
func NewWorkerPool(numWorkers int, extractor *extractor.Extractor, logger *slog.Logger) *WorkerPool {
	if numWorkers == 0 {
		numWorkers = util.GetOptimalPoolSize()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		numWorkers: numWorkers,
		jobs:       make(chan FileJob, numWorkers*2), // Buffered for smooth pipeline
		results:    make(chan FileResult, numWorkers),
		errors:     make(chan FileError, numWorkers),
		extractor:  extractor,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start spawns all worker goroutines.
//
// **IMPORTANT:** Must be called before submitting jobs.
//
// Workers will process jobs from the jobs channel until Stop() is called.
func (wp *WorkerPool) Start() {
	if !wp.started.CompareAndSwap(false, true) {
		wp.logger.Warn("WorkerPool already started")
		return
	}

	wp.logger.Info("Starting worker pool", "workers", wp.numWorkers)

	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// worker is the main worker goroutine function.
//
// Each worker:
//  1. Receives jobs from the jobs channel
//  2. Reads the file from disk
//  3. Calls extractor.ExtractFile()
//  4. Sends result or error to respective channels
//  5. Continues until jobs channel is closed or context cancelled
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	wp.logger.Debug("Worker started", "worker_id", id)

	for {
		select {
		case <-wp.ctx.Done():
			wp.logger.Debug("Worker cancelled", "worker_id", id)
			return

		case job, ok := <-wp.jobs:
			if !ok {
				// Jobs channel closed, worker exits
				wp.logger.Debug("Worker exiting (jobs closed)", "worker_id", id)
				return
			}

			// Process the job
			wp.logger.Debug("Worker received job", "worker_id", id, "file", job.FilePath, "job_id", job.JobID)
			wp.processJob(id, job)
			wp.logger.Debug("Worker finished job", "worker_id", id, "job_id", job.JobID)
		}
	}
}

// processJob processes a single file job.
func (wp *WorkerPool) processJob(workerID int, job FileJob) {
	wp.logger.Debug("Processing job - reading file", "worker_id", workerID, "file", job.FilePath)

	// Read file
	content, err := os.ReadFile(job.FilePath)
	if err != nil {
		wp.logger.Debug("File read error", "worker_id", workerID, "file", job.FilePath, "error", err)
		wp.jobsFailed.Add(1)
		wp.errors <- FileError{
			FilePath: job.FilePath,
			Error:    fmt.Errorf("failed to read file: %w", err),
		}
		return
	}

	wp.logger.Debug("Processing job - extracting", "worker_id", workerID, "file", job.FilePath, "size", len(content))

	// Extract symbols, imports, exports, call sites (single parse!)
	result, err := wp.extractor.ExtractFile(job.FilePath, content)
	if err != nil {
		wp.logger.Debug("Extraction error", "worker_id", workerID, "file", job.FilePath, "error", err)
		wp.jobsFailed.Add(1)
		wp.errors <- FileError{
			FilePath: job.FilePath,
			Error:    fmt.Errorf("extraction failed: %w", err),
		}
		return
	}

	wp.logger.Debug("Processing job - sending result", "worker_id", workerID, "file", job.FilePath, "symbols", len(result.Symbols))

	// Send result
	wp.jobsProcessed.Add(1)
	wp.results <- FileResult{
		FilePath: job.FilePath,
		Result:   result,
		JobID:    job.JobID,
	}

	wp.logger.Debug("Processing job - result sent", "worker_id", workerID, "job_id", job.JobID)
}

// Submit enqueues a job for processing.
//
// **Thread Safety:** Safe for concurrent calls.
//
// **Blocking:** Will block if jobs channel is full.
// Use context cancellation or timeout if needed.
func (wp *WorkerPool) Submit(job FileJob) error {
	if wp.stopped.Load() {
		return fmt.Errorf("worker pool is stopped")
	}

	wp.jobsSubmitted.Add(1)

	select {
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool cancelled")
	case wp.jobs <- job:
		return nil
	}
}

// Results returns the results channel.
//
// Consumers should read from this channel to collect processed results.
func (wp *WorkerPool) Results() <-chan FileResult {
	return wp.results
}

// Errors returns the errors channel.
//
// Consumers should read from this channel to collect errors.
func (wp *WorkerPool) Errors() <-chan FileError {
	return wp.errors
}

// FinishSubmitting closes the jobs channel to signal no more jobs will be submitted.
//
// **IMPORTANT:** Must be called after all jobs have been submitted and before
// waiting for results. This allows workers to exit gracefully when the jobs
// channel is drained.
//
// **Thread Safety:** Safe to call multiple times (idempotent).
//
// **Example:**
//
//	// Submit all jobs
//	for _, file := range files {
//	    pool.Submit(FileJob{FilePath: file})
//	}
//	pool.FinishSubmitting()  // Signal no more jobs
//
//	// Collect results...
func (wp *WorkerPool) FinishSubmitting() {
	wp.logger.Debug("FinishSubmitting called", "jobs_submitted", wp.jobsSubmitted.Load())
	// Only close once - use CAS to ensure thread safety
	if wp.jobsClosed.CompareAndSwap(false, true) {
		close(wp.jobs)
		wp.logger.Info("Jobs channel closed, no more jobs will be accepted", "total_submitted", wp.jobsSubmitted.Load())
	} else {
		wp.logger.Debug("Jobs channel already closed, skipping")
	}
}

// Wait blocks until all workers have finished.
//
// **Call this after:**
//  1. All jobs have been submitted
//  2. FinishSubmitting() has been called
//
// **Example:**
//
//	// Submit all jobs
//	for _, file := range files {
//	    pool.Submit(FileJob{FilePath: file})
//	}
//	pool.FinishSubmitting()  // Signal no more jobs
//
//	// Wait for workers to finish
//	pool.Wait()
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

// Stop gracefully shuts down the worker pool.
//
// **Steps:**
//  1. Closes jobs channel if not already closed (no new jobs accepted)
//  2. Waits for in-flight jobs to complete
//  3. Closes result and error channels
//
// **Thread Safety:** Safe to call multiple times (idempotent).
//
// **Note:** If FinishSubmitting() was already called, this will skip
// closing the jobs channel and proceed directly to cleanup.
func (wp *WorkerPool) Stop() {
	if !wp.stopped.CompareAndSwap(false, true) {
		return // Already stopped
	}

	wp.logger.Info("Stopping worker pool")

	// Signal workers to stop (close jobs channel if not already closed)
	if wp.jobsClosed.CompareAndSwap(false, true) {
		close(wp.jobs)
	}

	// Wait for all workers to finish
	wp.wg.Wait()

	// Close result and error channels
	close(wp.results)
	close(wp.errors)

	// Cancel context
	wp.cancel()

	wp.logger.Info("Worker pool stopped",
		"jobs_submitted", wp.jobsSubmitted.Load(),
		"jobs_processed", wp.jobsProcessed.Load(),
		"jobs_failed", wp.jobsFailed.Load())
}

// GetStats returns current worker pool statistics.
func (wp *WorkerPool) GetStats() WorkerPoolStats {
	return WorkerPoolStats{
		NumWorkers:     wp.numWorkers,
		JobsSubmitted:  wp.jobsSubmitted.Load(),
		JobsProcessed:  wp.jobsProcessed.Load(),
		JobsFailed:     wp.jobsFailed.Load(),
		QueueLength:    len(wp.jobs),
		ResultsQueued:  len(wp.results),
		ErrorsQueued:   len(wp.errors),
	}
}

// WorkerPoolStats contains statistics about the worker pool.
type WorkerPoolStats struct {
	NumWorkers     int
	JobsSubmitted  int64
	JobsProcessed  int64
	JobsFailed     int64
	QueueLength    int // Current jobs in queue
	ResultsQueued  int // Results waiting to be consumed
	ErrorsQueued   int // Errors waiting to be consumed
}

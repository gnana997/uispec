package util

import "runtime"

// GetOptimalPoolSize returns the optimal pool size for CPU-bound tasks.
//
// Formula: min(max(runtime.NumCPU() * 2, 4), 32)
//
// Reasoning:
//   - Minimum 4: Ensure some parallelism even on weak machines
//   - 2× CPU cores: Optimal for CGO-heavy workloads (allows parallelism during CGO blocks)
//   - Maximum 32: Scales for high-core machines while limiting memory (~32MB per language)
//
// Examples:
//   - 1-2 cores: 4 (minimum enforced)
//   - 4 cores: 8
//   - 8 cores: 16
//   - 16 cores: 32 (maximum enforced)
//   - 24 cores: 32 (capped to limit memory usage)
//
// This is used for:
//   - Parser pool size (parsers per language)
//   - Worker pool size (concurrent file processors)
//   - Any CPU-bound parallel processing
func GetOptimalPoolSize() int {
	cores := runtime.NumCPU()
	poolSize := cores * 2

	// Enforce minimum
	if poolSize < 4 {
		poolSize = 4
	}

	// Enforce maximum
	if poolSize > 32 {
		poolSize = 32
	}

	return poolSize
}

// GetOptimalPoolSizeWithOverride returns pool size with optional override.
//
// If override > 0, uses override value (for testing/tuning).
// Otherwise, uses GetOptimalPoolSize().
func GetOptimalPoolSizeWithOverride(override int) int {
	if override > 0 {
		return override
	}
	return GetOptimalPoolSize()
}

package parser

import (
	"github.com/gnana997/uispec/pkg/util"
)

// getDefaultPoolSize returns the default pool size based on CPU count.
//
// Delegates to util.GetOptimalPoolSize() to ensure consistency across
// all pools (parser pool, worker pool, etc).
//
// **CRITICAL:** This MUST match the worker pool size to prevent workers
// from blocking while waiting for available parsers.
//
// Pool sizing strategy:
// - Base: 2x CPU cores (allows parallelism during CGO-heavy operations)
// - Minimum: 4 parsers (ensures decent concurrency on low-end machines)
// - Maximum: 32 parsers (scales for high-core machines while limiting memory)
//
// Examples:
// - 1-2 cores → 4 parsers (minimum)
// - 4 cores → 8 parsers
// - 8 cores → 16 parsers
// - 16 cores → 32 parsers (maximum)
// - 24 cores → 32 parsers (capped to prevent over-provisioning)
//
// Memory Impact (per language):
// - 4 parsers: ~4MB
// - 8 parsers: ~8MB
// - 16 parsers: ~16MB
// - 32 parsers: ~32MB
// - Total (5 languages + TSX): ~48-192MB depending on CPU
func getDefaultPoolSize() int {
	return util.GetOptimalPoolSize()
}

// getPoolSize returns the pool size to use, allowing for override.
// If override is 0, returns the default based on CPU count.
// If override is > 0, uses the override value.
//
// This function is designed for future configurability without changing the API.
func getPoolSize(override int) int {
	return util.GetOptimalPoolSizeWithOverride(override)
}

// Tests for FileCache with mmap-based file access.
package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestFiles creates temporary test files for testing.
func setupTestFiles(t *testing.T) (dir string, files map[string]string) {
	t.Helper()

	dir = t.TempDir()
	files = make(map[string]string)

	// Small TypeScript file
	tsCode := `export class Calculator {
  add(a: number, b: number): number {
    return a + b;
  }
}`
	tsPath := filepath.Join(dir, "calc.ts")
	require.NoError(t, os.WriteFile(tsPath, []byte(tsCode), 0644))
	files["calc.ts"] = tsPath

	// Multi-line Python file
	pyCode := `def greet(name: str) -> str:
    """Greet someone by name."""
    return f"Hello, {name}!"

def farewell(name: str) -> str:
    """Say goodbye to someone."""
    return f"Goodbye, {name}!"`
	pyPath := filepath.Join(dir, "greet.py")
	require.NoError(t, os.WriteFile(pyPath, []byte(pyCode), 0644))
	files["greet.py"] = pyPath

	// Unicode file (emoji + Chinese)
	unicodeCode := `function greet(name: string): string {
  // 👋 Hello function
  return "Hello " + name + " 你好";
}`
	unicodePath := filepath.Join(dir, "unicode.ts")
	require.NoError(t, os.WriteFile(unicodePath, []byte(unicodeCode), 0644))
	files["unicode.ts"] = unicodePath

	// Empty file
	emptyPath := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(emptyPath, []byte{}, 0644))
	files["empty.txt"] = emptyPath

	// Large file (for performance testing)
	largeCode := strings.Repeat("// This is a comment line\n", 1000) // ~26KB
	largePath := filepath.Join(dir, "large.js")
	require.NoError(t, os.WriteFile(largePath, []byte(largeCode), 0644))
	files["large.js"] = largePath

	return dir, files
}

// TestFileCache_BasicOperations verifies core FileCache operations.
func TestFileCache_BasicOperations(t *testing.T) {
	_, files := setupTestFiles(t)
	tsPath := files["calc.ts"]

	// Create FileCache with default config
	cache := NewFileCache(DefaultFileCacheConfig())
	defer cache.Close()

	// Initial size should be 0
	assert.Equal(t, 0, cache.Size(), "Initial cache should be empty")

	// Get file (should load and mmap it)
	mf, err := cache.Get(tsPath)
	require.NoError(t, err)
	require.NotNil(t, mf)
	assert.Equal(t, tsPath, mf.Path)
	assert.NotNil(t, mf.Data)
	assert.Greater(t, mf.Size, int64(0))

	// Size should now be 1
	assert.Equal(t, 1, cache.Size(), "Cache should contain 1 file")

	// Get same file again (should hit cache)
	mf2, err := cache.Get(tsPath)
	require.NoError(t, err)
	assert.Equal(t, mf.Path, mf2.Path)

	// FetchCode using byte offsets
	// Extract "Calculator" from first line (starts at byte 13)
	code, err := cache.FetchCode(tsPath, 13, 23) // "Calculator"
	require.NoError(t, err)
	assert.Equal(t, "Calculator", code)

	// Extract entire first line
	code, err = cache.FetchCode(tsPath, 0, 30) // "export class Calculator {"
	require.NoError(t, err)
	assert.Contains(t, code, "export class Calculator")

	// Stats should show cache activity
	stats := cache.Stats()
	assert.Equal(t, 1, stats.FilesCached)
	assert.Greater(t, stats.CacheHits, int64(0)) // Second Get() was a hit
	assert.Equal(t, int64(1), stats.FilesLoaded)
	assert.Greater(t, stats.TotalMappedMB, float64(0))

	// Close should succeed
	err = cache.Close()
	assert.NoError(t, err)

	// Size should be 0 after close
	assert.Equal(t, 0, cache.Size())

	t.Logf("Cache stats: loaded=%d, hits=%d, misses=%d, mapped=%.2f MB",
		stats.FilesLoaded, stats.CacheHits, stats.CacheMisses, stats.TotalMappedMB)
}

// TestFileCache_Limits_MaxFiles verifies MaxFiles limit enforcement.
func TestFileCache_Limits_MaxFiles(t *testing.T) {
	dir := t.TempDir()

	// Create cache with MaxFiles=2
	config := &FileCacheConfig{
		MaxFiles:      2,
		MaxMemoryMB:   0, // Unlimited memory
		EnableMetrics: true,
	}
	cache := NewFileCache(config)
	defer cache.Close()

	// Create 3 test files
	file1 := filepath.Join(dir, "file1.txt")
	file2 := filepath.Join(dir, "file2.txt")
	file3 := filepath.Join(dir, "file3.txt")
	require.NoError(t, os.WriteFile(file1, []byte("content1"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("content2"), 0644))
	require.NoError(t, os.WriteFile(file3, []byte("content3"), 0644))

	// Load first 2 files - should succeed
	_, err := cache.Get(file1)
	require.NoError(t, err)
	_, err = cache.Get(file2)
	require.NoError(t, err)
	assert.Equal(t, 2, cache.Size())

	// Try to load 3rd file - should fail with limit error
	_, err = cache.Get(file3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FileCache limit reached")
	assert.Contains(t, err.Error(), "2 files")

	// Cache size should still be 2
	assert.Equal(t, 2, cache.Size())

	t.Logf("MaxFiles limit enforced correctly: %v", err)
}

// TestFileCache_Limits_MaxMemoryMB verifies MaxMemoryMB limit enforcement.
func TestFileCache_Limits_MaxMemoryMB(t *testing.T) {
	dir := t.TempDir()

	// Create cache with MaxMemoryMB=1 (1MB limit)
	config := &FileCacheConfig{
		MaxFiles:      0, // Unlimited files
		MaxMemoryMB:   1, // 1MB memory limit
		EnableMetrics: true,
	}
	cache := NewFileCache(config)
	defer cache.Close()

	// Create a small file (0.5MB) - should succeed
	smallContent := strings.Repeat("x", 512*1024) // 0.5MB
	smallPath := filepath.Join(dir, "small.txt")
	require.NoError(t, os.WriteFile(smallPath, []byte(smallContent), 0644))

	_, err := cache.Get(smallPath)
	require.NoError(t, err) // Should succeed (under limit)

	// Create another file (0.6MB) - total would be 1.1MB, should fail
	mediumContent := strings.Repeat("y", 614*1024) // 0.6MB
	mediumPath := filepath.Join(dir, "medium.txt")
	require.NoError(t, os.WriteFile(mediumPath, []byte(mediumContent), 0644))

	// Try to load medium file - should fail with memory limit error
	_, err = cache.Get(mediumPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FileCache memory limit reached")
	assert.Contains(t, err.Error(), "1 MB")

	t.Logf("MaxMemoryMB limit enforced correctly: %v", err)
}

// TestFileCache_Unbounded verifies unbounded cache works with many files.
func TestFileCache_Unbounded(t *testing.T) {
	dir := t.TempDir()

	// Create cache with no limits
	cache := NewFileCache(UnboundedFileCacheConfig())
	defer cache.Close()

	// Create and load 100 files
	numFiles := 100
	for i := 0; i < numFiles; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		content := fmt.Sprintf("content of file %d", i)
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))

		_, err := cache.Get(path)
		require.NoError(t, err)
	}

	// All files should be cached
	assert.Equal(t, numFiles, cache.Size())

	stats := cache.Stats()
	assert.Equal(t, numFiles, stats.FilesCached)
	assert.Equal(t, int64(numFiles), stats.FilesLoaded)

	t.Logf("Unbounded cache: loaded %d files, mapped %.2f MB",
		stats.FilesCached, stats.TotalMappedMB)
}

// TestFileCache_ConcurrentAccess verifies thread-safe concurrent access.
func TestFileCache_ConcurrentAccess(t *testing.T) {
	_, files := setupTestFiles(t)
	tsPath := files["calc.ts"]
	pyPath := files["greet.py"]

	cache := NewFileCache(DefaultFileCacheConfig())
	defer cache.Close()

	// Launch 100 goroutines reading the same files
	numGoroutines := 100
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Alternate between two files
			path := tsPath
			if id%2 == 0 {
				path = pyPath
			}

			// Get file
			mf, err := cache.Get(path)
			if err != nil {
				errors <- fmt.Errorf("goroutine %d Get failed: %w", id, err)
				return
			}

			// FetchCode
			if len(mf.Data) > 10 {
				_, err = cache.FetchCode(path, 0, 10)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d FetchCode failed: %w", id, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Verify cache stats
	stats := cache.Stats()
	assert.Equal(t, 2, stats.FilesCached) // Only 2 unique files
	assert.Greater(t, stats.CacheHits, int64(90)) // Most accesses should be hits

	t.Logf("Concurrent access: %d goroutines, %d hits, %d misses",
		numGoroutines, stats.CacheHits, stats.CacheMisses)
}

// TestFileCache_ByteOffsetValidation verifies byte offset validation.
func TestFileCache_ByteOffsetValidation(t *testing.T) {
	_, files := setupTestFiles(t)
	tsPath := files["calc.ts"]

	cache := NewFileCache(DefaultFileCacheConfig())
	defer cache.Close()

	// Get file to load it
	mf, err := cache.Get(tsPath)
	require.NoError(t, err)
	fileSize := uint32(len(mf.Data))

	tests := []struct {
		name      string
		start     uint32
		end       uint32
		shouldErr bool
		errMsg    string
	}{
		{
			name:      "valid range",
			start:     0,
			end:       10,
			shouldErr: false,
		},
		{
			name:      "end before start",
			start:     10,
			end:       5,
			shouldErr: true,
			errMsg:    "invalid byte range",
		},
		{
			name:      "end equals start",
			start:     10,
			end:       10,
			shouldErr: true,
			errMsg:    "invalid byte range",
		},
		{
			name:      "end beyond file size",
			start:     0,
			end:       fileSize + 100,
			shouldErr: true,
			errMsg:    "invalid byte range",
		},
		{
			name:      "start at file end",
			start:     fileSize,
			end:       fileSize + 1,
			shouldErr: true,
			errMsg:    "invalid byte range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cache.FetchCode(tsPath, tt.start, tt.end)
			if tt.shouldErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestFileCache_UnicodeHandling verifies byte offsets work with Unicode.
func TestFileCache_UnicodeHandling(t *testing.T) {
	_, files := setupTestFiles(t)
	unicodePath := files["unicode.ts"]

	cache := NewFileCache(DefaultFileCacheConfig())
	defer cache.Close()

	// Get file
	mf, err := cache.Get(unicodePath)
	require.NoError(t, err)

	// File contains: "function greet(name: string): string {\n  // 👋 Hello function\n  return \"Hello \" + name + \" 你好\";\n}"
	// The emoji 👋 is 4 bytes, Chinese characters are 3 bytes each

	// Extract "greet" from first line (should work with byte offsets)
	code, err := cache.FetchCode(unicodePath, 9, 14) // "greet"
	require.NoError(t, err)
	assert.Equal(t, "greet", code)

	// Extract the entire function
	code, err = cache.FetchCode(unicodePath, 0, uint32(len(mf.Data)))
	require.NoError(t, err)
	assert.Contains(t, code, "👋")
	assert.Contains(t, code, "你好")

	t.Logf("Unicode file size: %d bytes, extracted successfully", len(mf.Data))
}

// TestFileCache_EmptyFiles verifies handling of empty files.
func TestFileCache_EmptyFiles(t *testing.T) {
	_, files := setupTestFiles(t)
	emptyPath := files["empty.txt"]

	cache := NewFileCache(DefaultFileCacheConfig())
	defer cache.Close()

	// Get empty file - should succeed
	mf, err := cache.Get(emptyPath)
	require.NoError(t, err)
	assert.Equal(t, int64(0), mf.Size)
	assert.Nil(t, mf.Data) // Data should be nil for empty files

	// FetchCode with 0,0 should work
	code, err := cache.FetchCode(emptyPath, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, "", code)

	// FetchCode with non-zero range should fail
	_, err = cache.FetchCode(emptyPath, 0, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid byte range for empty file")
}

// TestFileCache_ResourceCleanup verifies Close() releases resources.
func TestFileCache_ResourceCleanup(t *testing.T) {
	_, files := setupTestFiles(t)
	tsPath := files["calc.ts"]

	cache := NewFileCache(DefaultFileCacheConfig())

	// Load file
	_, err := cache.Get(tsPath)
	require.NoError(t, err)
	assert.Equal(t, 1, cache.Size())

	// Close cache
	err = cache.Close()
	assert.NoError(t, err)

	// Cache should be empty after close
	assert.Equal(t, 0, cache.Size())

	// Trying to use cache after close should fail (reload file)
	// Note: This actually works because Get() will reload the file
	// But the previous mmap'd regions should be unmapped
	_, err = cache.Get(tsPath)
	require.NoError(t, err) // Will reload

	// Close again
	err = cache.Close()
	assert.NoError(t, err)
}

// TestFileCache_StatsAccuracy verifies stats tracking is accurate.
func TestFileCache_StatsAccuracy(t *testing.T) {
	dir, files := setupTestFiles(t)
	tsPath := files["calc.ts"]
	pyPath := files["greet.py"]

	cache := NewFileCache(DefaultFileCacheConfig())
	defer cache.Close()

	// Initial stats
	stats := cache.Stats()
	assert.Equal(t, 0, stats.FilesCached)
	assert.Equal(t, int64(0), stats.FilesLoaded)
	assert.Equal(t, int64(0), stats.CacheHits)
	assert.Equal(t, int64(0), stats.CacheMisses)

	// Load first file (miss + load)
	_, err := cache.Get(tsPath)
	require.NoError(t, err)

	stats = cache.Stats()
	assert.Equal(t, 1, stats.FilesCached)
	assert.Equal(t, int64(1), stats.FilesLoaded)
	assert.Equal(t, int64(0), stats.CacheHits) // First access is a miss

	// Access same file again (hit)
	_, err = cache.Get(tsPath)
	require.NoError(t, err)

	stats = cache.Stats()
	assert.Equal(t, 1, stats.FilesCached)
	assert.Equal(t, int64(1), stats.FilesLoaded) // No new load
	assert.Greater(t, stats.CacheHits, int64(0)) // Should have hits now

	// Load second file (miss + load)
	_, err = cache.Get(pyPath)
	require.NoError(t, err)

	stats = cache.Stats()
	assert.Equal(t, 2, stats.FilesCached)
	assert.Equal(t, int64(2), stats.FilesLoaded)

	// Access both files multiple times
	for i := 0; i < 10; i++ {
		cache.Get(tsPath)
		cache.Get(pyPath)
	}

	stats = cache.Stats()
	assert.Equal(t, 2, stats.FilesCached)
	assert.Equal(t, int64(2), stats.FilesLoaded) // No new loads
	assert.Greater(t, stats.CacheHits, int64(15)) // Many hits

	// Try to load non-existent file (miss, no load)
	nonExistentPath := filepath.Join(dir, "nonexistent.txt")
	_, err = cache.Get(nonExistentPath)
	require.Error(t, err)

	stats = cache.Stats()
	assert.Equal(t, 2, stats.FilesCached) // Still 2 cached files
	// Misses might have increased (implementation-dependent)

	t.Logf("Final stats: cached=%d, loaded=%d, hits=%d, misses=%d, mapped=%.2f MB",
		stats.FilesCached, stats.FilesLoaded, stats.CacheHits, stats.CacheMisses, stats.TotalMappedMB)
}

// TestFileCache_FileNotFound verifies error handling for missing files.
func TestFileCache_FileNotFound(t *testing.T) {
	cache := NewFileCache(DefaultFileCacheConfig())
	defer cache.Close()

	// Try to get non-existent file
	_, err := cache.Get("/nonexistent/path/file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to") // Can be "failed to stat" or "failed to open"

	// Try to fetch code from non-existent file
	_, err = cache.FetchCode("/nonexistent/path/file.txt", 0, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to")
}

// BenchmarkFileCache_VsReadFile compares mmap vs os.ReadFile for random access.
func BenchmarkFileCache_VsReadFile(b *testing.B) {
	dir := b.TempDir()

	// Create 10 test files, each 10KB
	numFiles := 10
	files := make([]string, numFiles)
	for i := 0; i < numFiles; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		content := strings.Repeat(fmt.Sprintf("line %d content\n", i), 500) // ~10KB
		require.NoError(b, os.WriteFile(path, []byte(content), 0644))
		files[i] = path
	}

	b.Run("FileCache_mmap", func(b *testing.B) {
		cache := NewFileCache(DefaultFileCacheConfig())
		defer cache.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			path := files[i%numFiles]
			_, err := cache.FetchCode(path, 0, 100) // Read first 100 bytes
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ReadFile", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			path := files[i%numFiles]
			data, err := os.ReadFile(path)
			if err != nil {
				b.Fatal(err)
			}
			_ = string(data[0:100]) // Extract first 100 bytes
		}
	})
}

// BenchmarkFileCache_VsLineIteration compares byte slicing vs line splitting.
func BenchmarkFileCache_VsLineIteration(b *testing.B) {
	dir := b.TempDir()

	// Create file with 1000 lines
	lines := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		lines[i] = fmt.Sprintf("// Line %d: This is a comment line with some content", i)
	}
	content := strings.Join(lines, "\n")
	path := filepath.Join(dir, "test.js")
	require.NoError(b, os.WriteFile(path, []byte(content), 0644))

	// We want to extract lines 500-510 (10 lines from the middle)
	startLine := 500
	endLine := 510

	// Calculate byte offsets for lines 500-510
	startByte := 0
	for i := 0; i < startLine; i++ {
		startByte += len(lines[i]) + 1 // +1 for newline
	}
	endByte := startByte
	for i := startLine; i < endLine; i++ {
		endByte += len(lines[i]) + 1
	}

	b.Run("FileCache_ByteOffset", func(b *testing.B) {
		cache := NewFileCache(DefaultFileCacheConfig())
		defer cache.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := cache.FetchCode(path, uint32(startByte), uint32(endByte))
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("LineIteration", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			data, err := os.ReadFile(path)
			if err != nil {
				b.Fatal(err)
			}
			fileLines := strings.Split(string(data), "\n")
			_ = strings.Join(fileLines[startLine:endLine], "\n")
		}
	})
}

// BenchmarkFileCache_LargeFiles tests performance with large files.
func BenchmarkFileCache_LargeFiles(b *testing.B) {
	dir := b.TempDir()

	// Create 100KB file
	largeContent := strings.Repeat("// This is a comment line\n", 4000) // ~100KB
	path := filepath.Join(dir, "large.js")
	require.NoError(b, os.WriteFile(path, []byte(largeContent), 0644))

	cache := NewFileCache(DefaultFileCacheConfig())
	defer cache.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Read random 1KB chunks
		offset := uint32((i * 1024) % (len(largeContent) - 1024))
		_, err := cache.FetchCode(path, offset, offset+1024)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	stats := cache.Stats()
	b.Logf("Large file benchmark: hits=%d, misses=%d, mapped=%.2f MB",
		stats.CacheHits, stats.CacheMisses, stats.TotalMappedMB)
}

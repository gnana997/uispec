package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverFiles_BasicDirectory(t *testing.T) {
	files, err := DiscoverFiles("testdata", DefaultScanConfig())
	require.NoError(t, err)
	assert.Greater(t, len(files), 0, "should discover component files")

	// All results should be absolute paths.
	for _, f := range files {
		assert.True(t, filepath.IsAbs(f), "expected absolute path, got %s", f)
	}

	// Should include known fixture files.
	names := fileNames(files)
	assert.Contains(t, names, "button.tsx")
	assert.Contains(t, names, "arrow.tsx")
	assert.Contains(t, names, "utility.ts")
}

func TestDiscoverFiles_ExcludesTestFiles(t *testing.T) {
	// Create a temp dir with test/story files that should be excluded.
	tmp := t.TempDir()
	writeFile(t, tmp, "button.tsx", "export function Button() {}")
	writeFile(t, tmp, "button.test.tsx", "test('button', () => {})")
	writeFile(t, tmp, "button.spec.tsx", "describe('button', () => {})")
	writeFile(t, tmp, "button.stories.tsx", "export default { title: 'Button' }")
	writeFile(t, tmp, "button.story.tsx", "export default { title: 'Button' }")
	os.MkdirAll(filepath.Join(tmp, "__tests__"), 0755)
	writeFile(t, filepath.Join(tmp, "__tests__"), "utils.ts", "export {}")

	files, err := DiscoverFiles(tmp, DefaultScanConfig())
	require.NoError(t, err)

	names := fileNames(files)
	assert.Contains(t, names, "button.tsx")
	assert.NotContains(t, names, "button.test.tsx")
	assert.NotContains(t, names, "button.spec.tsx")
	assert.NotContains(t, names, "button.stories.tsx")
	assert.NotContains(t, names, "button.story.tsx")
	assert.NotContains(t, names, "utils.ts", "files inside __tests__/ should be excluded")
}

func TestDiscoverFiles_SortedOutput(t *testing.T) {
	files, err := DiscoverFiles("testdata", DefaultScanConfig())
	require.NoError(t, err)
	require.Greater(t, len(files), 1)

	for i := 1; i < len(files); i++ {
		assert.LessOrEqual(t, files[i-1], files[i], "files should be sorted")
	}
}

func TestDiscoverFiles_EmptyDirectory(t *testing.T) {
	tmp := t.TempDir()
	files, err := DiscoverFiles(tmp, DefaultScanConfig())
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestDiscoverFiles_InvalidGlob(t *testing.T) {
	cfg := DefaultScanConfig()
	cfg.Exclude = append(cfg.Exclude, "[invalid")
	_, err := DiscoverFiles("testdata", cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid exclude pattern")
}

// --- helpers ---

func fileNames(paths []string) []string {
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}
	return names
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

package scanner

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
)

func setupExtractor(t *testing.T) (*extractor.Extractor, func()) {
	t.Helper()
	pm := parser.NewParserManager(nil)
	qm := queries.NewQueryManager(pm, nil)
	ext := extractor.NewExtractor(pm, qm, nil)
	return ext, func() {
		qm.Close()
		pm.Close()
	}
}

func absTestdata(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", name))
	require.NoError(t, err)
	return abs
}

func TestExtractAll_SingleFile(t *testing.T) {
	ext, cleanup := setupExtractor(t)
	defer cleanup()

	files := []string{absTestdata(t, "button.tsx")}
	results, failed := ExtractAll(files, ext, nil)

	assert.Equal(t, 0, failed)
	require.Len(t, results, 1)
	assert.Equal(t, files[0], results[0].FilePath)
	assert.NotEmpty(t, results[0].SourceCode, "source bytes should be preserved")
	assert.NotNil(t, results[0].Result)
	assert.Greater(t, len(results[0].Result.Symbols), 0, "should extract symbols")
	assert.Greater(t, len(results[0].Result.Exports), 0, "should extract exports")
}

func TestExtractAll_MultipleFiles(t *testing.T) {
	ext, cleanup := setupExtractor(t)
	defer cleanup()

	files := []string{
		absTestdata(t, "button.tsx"),
		absTestdata(t, "arrow.tsx"),
		absTestdata(t, "utility.ts"),
	}
	results, failed := ExtractAll(files, ext, nil)

	assert.Equal(t, 0, failed)
	assert.Len(t, results, 3)
}

func TestExtractAll_HandlesErrors(t *testing.T) {
	ext, cleanup := setupExtractor(t)
	defer cleanup()

	files := []string{
		absTestdata(t, "button.tsx"),
		"/nonexistent/file.tsx",
	}
	results, failed := ExtractAll(files, ext, nil)

	assert.Equal(t, 1, failed)
	assert.Len(t, results, 1, "valid file should still succeed")
}

func TestExtractAll_EmptyInput(t *testing.T) {
	ext, cleanup := setupExtractor(t)
	defer cleanup()

	results, failed := ExtractAll(nil, ext, nil)
	assert.Equal(t, 0, failed)
	assert.Empty(t, results)
}

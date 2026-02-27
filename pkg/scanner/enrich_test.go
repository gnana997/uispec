package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindNodeRuntime(t *testing.T) {
	rt, found := findNodeRuntime()
	if !found {
		t.Skip("no node or bun runtime available")
	}
	assert.NotEmpty(t, rt)
}

func TestFindTSConfig_Found(t *testing.T) {
	// Create a temp dir with a tsconfig.json.
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0644)
	require.NoError(t, err)

	path, found := findTSConfig(dir)
	assert.True(t, found)
	assert.Equal(t, filepath.Join(dir, "tsconfig.json"), path)
}

func TestFindTSConfig_FoundInParent(t *testing.T) {
	// Create parent with tsconfig, child without.
	parent := t.TempDir()
	child := filepath.Join(parent, "src", "components")
	require.NoError(t, os.MkdirAll(child, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "tsconfig.json"), []byte("{}"), 0644))

	path, found := findTSConfig(child)
	assert.True(t, found)
	assert.Equal(t, filepath.Join(parent, "tsconfig.json"), path)
}

func TestFindTSConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, found := findTSConfig(dir)
	assert.False(t, found)
}

func TestCheckNodeModules(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, checkNodeModules(dir))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules"), 0755))
	assert.True(t, checkNodeModules(dir))
}

func TestCheckNodeModules_InParent(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "src")
	require.NoError(t, os.MkdirAll(child, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "node_modules"), 0755))

	assert.True(t, checkNodeModules(child))
}

func TestDocgenScript_Embedded(t *testing.T) {
	// Verify the embedded script is non-empty.
	assert.True(t, len(docgenScript) > 0, "embedded docgen script should not be empty")
}

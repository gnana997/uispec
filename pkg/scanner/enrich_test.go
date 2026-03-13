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

func TestFindAllNodeModules(t *testing.T) {
	dir := t.TempDir()
	assert.Empty(t, findAllNodeModules(dir))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules"), 0755))
	dirs := findAllNodeModules(dir)
	assert.Len(t, dirs, 1)
	assert.Equal(t, filepath.Join(dir, "node_modules"), dirs[0])
}

func TestFindAllNodeModules_Monorepo(t *testing.T) {
	// Simulates: root/node_modules + root/packages/ui/node_modules
	root := t.TempDir()
	pkgDir := filepath.Join(root, "packages", "ui")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "node_modules"), 0755))

	dirs := findAllNodeModules(pkgDir)
	assert.Len(t, dirs, 2)
	// Nearest first.
	assert.Equal(t, filepath.Join(pkgDir, "node_modules"), dirs[0])
	assert.Equal(t, filepath.Join(root, "node_modules"), dirs[1])
}

func TestFindTypescriptDir(t *testing.T) {
	dir := t.TempDir()
	nm := filepath.Join(dir, "node_modules")
	tsLib := filepath.Join(nm, "typescript", "lib")
	require.NoError(t, os.MkdirAll(tsLib, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tsLib, "typescript.js"), []byte(""), 0644))

	assert.Equal(t, nm, findTypescriptDir([]string{nm}))
	assert.Empty(t, findTypescriptDir([]string{filepath.Join(dir, "other")}))
}

func TestDocgenScript_Embedded(t *testing.T) {
	// Verify the embedded script is non-empty.
	assert.True(t, len(docgenScript) > 0, "embedded docgen script should not be empty")
}

package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokensScript_Embedded(t *testing.T) {
	assert.True(t, len(tokensScript) > 0, "embedded tokens script should not be empty")
}

func TestDiscoverCSSFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some CSS files and non-CSS files.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "globals.css"), []byte(":root {}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "theme.css"), []byte(":root {}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.tsx"), []byte("export default function App() {}"), 0644))

	// Create a node_modules dir with a CSS file that should be excluded.
	nmDir := filepath.Join(dir, "node_modules", "some-lib")
	require.NoError(t, os.MkdirAll(nmDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nmDir, "style.css"), []byte(":root {}"), 0644))

	excludes := []string{"node_modules/**"}
	files, err := DiscoverCSSFiles(dir, excludes)
	require.NoError(t, err)

	assert.Len(t, files, 2)
	for _, f := range files {
		assert.True(t, filepath.Ext(f) == ".css")
		assert.NotContains(t, f, "node_modules")
	}
}

func TestDiscoverCSSFiles_Empty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.tsx"), []byte("export default function App() {}"), 0644))

	files, err := DiscoverCSSFiles(dir, nil)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestRunTokenExtraction_Integration(t *testing.T) {
	rt, found := findNodeRuntime()
	if !found {
		t.Skip("no node or bun runtime available")
	}

	if len(tokensScript) == 0 {
		t.Skip("tokens script not embedded (run make docgen-bundle first)")
	}

	dir := t.TempDir()

	// Create a globals.css with :root tokens and .dark block.
	css := `:root {
  --radius: 0.625rem;
  --background: oklch(1 0 0);
  --foreground: oklch(0.145 0 0);
  --primary: oklch(0.205 0 0);
  --primary-foreground: oklch(0.985 0 0);
  --chart-1: oklch(0.646 0.222 41.116);
  --sidebar-background: oklch(0.985 0 0);
  --tw-ring-offset: 0;
}

.dark {
  --background: oklch(0.145 0 0);
  --foreground: oklch(0.985 0 0);
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "globals.css"), []byte(css), 0644))

	result, err := RunTokenExtraction(dir, []string{filepath.Join(dir, "globals.css")}, rt, nil)
	require.NoError(t, err)

	// Should detect dark mode.
	assert.True(t, result.DarkMode)

	// Should extract tokens from :root only (not .dark).
	assert.GreaterOrEqual(t, len(result.Tokens), 6)

	// Check specific tokens exist with correct categories.
	tokenMap := make(map[string]tokenResult)
	for _, tok := range result.Tokens {
		tokenMap[tok.Name] = tok
	}

	// Verify known tokens.
	bg, ok := tokenMap["background"]
	assert.True(t, ok, "should have background token")
	assert.Equal(t, "color", bg.Category)
	assert.Equal(t, "oklch(1 0 0)", bg.Value)

	primary, ok := tokenMap["primary"]
	assert.True(t, ok, "should have primary token")
	assert.Equal(t, "color", primary.Category)

	radius, ok := tokenMap["radius"]
	assert.True(t, ok, "should have radius token")
	assert.Equal(t, "border", radius.Category)
	assert.Equal(t, "0.625rem", radius.Value)

	chart, ok := tokenMap["chart-1"]
	assert.True(t, ok, "should have chart-1 token")
	assert.Equal(t, "chart", chart.Category)

	sidebar, ok := tokenMap["sidebar-background"]
	assert.True(t, ok, "should have sidebar-background token")
	assert.Equal(t, "sidebar", sidebar.Category)

	// Should NOT have tw- internal variables.
	_, hasTw := tokenMap["tw-ring-offset"]
	assert.False(t, hasTw, "should filter out --tw-* variables")
}

func TestRunTokenExtraction_ThemeHints(t *testing.T) {
	rt, found := findNodeRuntime()
	if !found {
		t.Skip("no node or bun runtime available")
	}

	if len(tokensScript) == 0 {
		t.Skip("tokens script not embedded (run make docgen-bundle first)")
	}

	dir := t.TempDir()

	// CSS with both :root and @theme (Tailwind v4 pattern).
	css := `:root {
  --background: oklch(1 0 0);
  --foreground: oklch(0.145 0 0);
  --border: oklch(0.922 0 0);
}

@theme inline {
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-border: var(--border);
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "globals.css"), []byte(css), 0644))

	result, err := RunTokenExtraction(dir, []string{filepath.Join(dir, "globals.css")}, rt, nil)
	require.NoError(t, err)

	tokenMap := make(map[string]tokenResult)
	for _, tok := range result.Tokens {
		tokenMap[tok.Name] = tok
	}

	// --border would match "border" category by name pattern,
	// but @theme confirms it's --color-border → "color" category.
	border, ok := tokenMap["border"]
	assert.True(t, ok, "should have border token")
	assert.Equal(t, "color", border.Category, "@theme hint should override name pattern")
}

func TestRunTokenExtraction_EmptyFiles(t *testing.T) {
	rt, found := findNodeRuntime()
	if !found {
		t.Skip("no node or bun runtime available")
	}

	result, err := RunTokenExtraction(t.TempDir(), nil, rt, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Tokens)
}

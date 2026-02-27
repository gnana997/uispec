package scanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "embed"

	"github.com/bmatcuk/doublestar/v4"
)

//go:embed scripts/dist/tokens-worker.js
var tokensScript []byte

// findProjectRoot walks up from dir looking for package.json to find the project root.
// CSS files (globals.css, styles/) typically live at the project root, not in
// a component subdirectory, so token extraction needs to search from there.
// Falls back to dir itself if no package.json is found.
func findProjectRoot(dir string) string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}

	orig := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return orig
}

// DiscoverCSSFiles walks rootDir for .css files, applying exclude patterns.
func DiscoverCSSFiles(rootDir string, excludes []string) ([]string, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root path: %w", err)
	}

	var files []string

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)

		// Check exclusions.
		for _, pattern := range excludes {
			matched, _ := doublestar.PathMatch(pattern, relPath)
			if matched {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if d.IsDir() {
			return nil
		}

		// Only .css files.
		if strings.HasSuffix(strings.ToLower(path), ".css") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

// RunTokenExtraction executes the PostCSS token worker to extract design tokens from CSS files.
func RunTokenExtraction(rootDir string, cssFiles []string, runtime string, log *slog.Logger) (*TokenExtractionResult, error) {
	if log == nil {
		log = slog.Default()
	}

	if len(cssFiles) == 0 {
		return &TokenExtractionResult{}, nil
	}

	start := time.Now()

	// Write the embedded script to a temp file.
	tmpFile, err := os.CreateTemp("", "uispec-tokens-*.js")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(tokensScript); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("failed to write tokens script: %w", err)
	}
	tmpFile.Close()

	// Prepare input JSON.
	input := tokenInput{
		CSSFiles: cssFiles,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	// Build command.
	args := []string{tmpFile.Name()}
	cmd := exec.Command(runtime, args...)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = rootDir

	log.Info("running token extraction",
		"runtime", filepath.Base(runtime),
		"cssFiles", len(cssFiles))

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			log.Warn("tokens worker stderr", "output", stderrStr)
		}
		return nil, fmt.Errorf("tokens worker failed: %w (stderr: %s)", err, stderrStr)
	}

	// Parse output.
	var output tokenWorkerOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, fmt.Errorf("failed to parse tokens output: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	log.Info("token extraction complete",
		"tokens", len(output.Tokens),
		"darkMode", output.DarkMode,
		"runtime", filepath.Base(runtime),
		"ms", duration)

	return &TokenExtractionResult{
		Tokens:     output.Tokens,
		DarkMode:   output.DarkMode,
		Runtime:    filepath.Base(runtime),
		DurationMs: duration,
	}, nil
}

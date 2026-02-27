package scanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "embed"
)

//go:embed scripts/dist/docgen-worker.js
var docgenScript []byte

// EnrichConfig holds configuration for the Node.js enrichment phase.
type EnrichConfig struct {
	// RootDir is the project root (where tsconfig.json lives).
	RootDir string
	// Files are the absolute paths to component files to enrich.
	Files []string
}

// EnrichResult holds the output of the Node.js enrichment phase.
type EnrichResult struct {
	// Components maps displayName to its docgen result.
	Components map[string]*DocgenResult
	// Runtime is the Node runtime that was used ("node" or "bun").
	Runtime string
	// DurationMs is how long the enrichment took.
	DurationMs int64
}

// findNodeRuntime searches for a Node.js runtime on the PATH.
// Prefers bun over node for speed.
func findNodeRuntime() (string, bool) {
	for _, rt := range []string{"bun", "node"} {
		if p, err := exec.LookPath(rt); err == nil {
			return p, true
		}
	}
	return "", false
}

// findTSConfig searches for tsconfig.json starting at dir and walking up.
func findTSConfig(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}

	for {
		candidate := filepath.Join(dir, "tsconfig.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// checkNodeModules checks if node_modules exists at or above dir.
func checkNodeModules(dir string) bool {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}

	for {
		candidate := filepath.Join(dir, "node_modules")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}

// CanEnrich checks whether Node.js enrichment is available for the given directory.
// Returns the tsconfig path and runtime path if enrichment is possible.
func CanEnrich(rootDir string, log *slog.Logger) (tsconfig string, runtime string, ok bool) {
	rt, found := findNodeRuntime()
	if !found {
		log.Debug("enrichment skipped: no node or bun runtime found on PATH")
		return "", "", false
	}

	tsconfig, found = findTSConfig(rootDir)
	if !found {
		log.Debug("enrichment skipped: no tsconfig.json found", "dir", rootDir)
		return "", "", false
	}

	if !checkNodeModules(rootDir) {
		log.Debug("enrichment skipped: no node_modules found", "dir", rootDir)
		return "", "", false
	}

	return tsconfig, rt, true
}

// RunEnrich executes the Node.js docgen worker to extract enriched prop data.
func RunEnrich(cfg EnrichConfig, runtime string, tsconfig string, log *slog.Logger) (*EnrichResult, error) {
	if len(cfg.Files) == 0 {
		return &EnrichResult{Components: make(map[string]*DocgenResult)}, nil
	}

	start := time.Now()

	// Write the embedded script to a temp file.
	tmpFile, err := os.CreateTemp("", "uispec-docgen-*.js")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.Write(docgenScript); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("failed to write docgen script: %w", err)
	}
	_ = tmpFile.Close()

	// Prepare input JSON.
	input := docgenInput{
		Files:    cfg.Files,
		TSConfig: tsconfig,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	// Build command with memory limit.
	args := []string{tmpFile.Name()}
	if strings.HasSuffix(filepath.Base(runtime), "node") || strings.Contains(runtime, "node") {
		args = append([]string{"--max-old-space-size=2048"}, args...)
	}

	cmd := exec.Command(runtime, args...)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set working directory to root for correct path resolution.
	cmd.Dir = cfg.RootDir

	log.Info("running enrichment",
		"runtime", filepath.Base(runtime),
		"files", len(cfg.Files),
		"tsconfig", tsconfig)

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			log.Warn("docgen worker stderr", "output", stderrStr)
		}
		return nil, fmt.Errorf("docgen worker failed: %w (stderr: %s)", err, stderrStr)
	}

	// Parse output.
	var results []DocgenResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil, fmt.Errorf("failed to parse docgen output: %w", err)
	}

	// Index by displayName.
	components := make(map[string]*DocgenResult, len(results))
	for i := range results {
		components[results[i].DisplayName] = &results[i]
	}

	duration := time.Since(start).Milliseconds()
	log.Info("enrichment complete",
		"components", len(results),
		"runtime", filepath.Base(runtime),
		"ms", duration)

	return &EnrichResult{
		Components: components,
		Runtime:    filepath.Base(runtime),
		DurationMs: duration,
	}, nil
}

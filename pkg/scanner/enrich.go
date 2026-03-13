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

//go:embed scripts/dist/docgen-worker.cjs
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

// findAllNodeModules collects all node_modules directories from dir upward.
// This handles monorepos where typescript may be hoisted to a root node_modules
// while the scanned package has its own nested node_modules.
func findAllNodeModules(dir string) []string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}

	var dirs []string
	for {
		candidate := filepath.Join(dir, "node_modules")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			dirs = append(dirs, candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dirs
}

// findTypescriptDir returns the node_modules directory that contains typescript,
// searching all node_modules from dir upward. Returns empty string if not found.
func findTypescriptDir(dirs []string) string {
	for _, d := range dirs {
		tsPath := filepath.Join(d, "typescript", "lib", "typescript.js")
		if _, err := os.Stat(tsPath); err == nil {
			return d
		}
	}
	return ""
}

// resolveGlobalTypescript uses the Node runtime to find a globally installed typescript.
// Returns the directory containing typescript, or empty string if not found.
func resolveGlobalTypescript(runtime string) string {
	cmd := exec.Command(runtime, "-e", "console.log(require.resolve('typescript'))")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// require.resolve returns something like /usr/lib/node_modules/typescript/lib/typescript.js
	// We need the parent node_modules dir.
	resolved := strings.TrimSpace(string(out))
	// Walk up to find the node_modules dir containing typescript.
	parts := strings.Split(resolved, string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "node_modules" {
			return string(filepath.Separator) + filepath.Join(parts[:i+1]...)
		}
	}
	return ""
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

	nmDirs := findAllNodeModules(rootDir)
	if len(nmDirs) == 0 {
		log.Debug("enrichment skipped: no node_modules found", "dir", rootDir)
		return "", "", false
	}

	// Check if typescript is available in any discovered node_modules.
	tsDir := findTypescriptDir(nmDirs)
	if tsDir == "" {
		// Try global fallback.
		globalDir := resolveGlobalTypescript(rt)
		if globalDir == "" {
			log.Warn("enrichment skipped: typescript not found in node_modules",
				"searched", nmDirs,
				"hint", "install it: npm install typescript")
			return "", "", false
		}
		log.Info("using globally installed typescript", "path", globalDir)
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
	tmpFile, err := os.CreateTemp("", "uispec-docgen-*.cjs")
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

	// Set NODE_PATH with all discovered node_modules directories so the worker
	// can find typescript. This handles monorepos where typescript may be hoisted
	// to a root node_modules above the scanned directory.
	nmDirs := findAllNodeModules(cfg.RootDir)
	if globalDir := resolveGlobalTypescript(runtime); globalDir != "" {
		nmDirs = append(nmDirs, globalDir)
	}
	if len(nmDirs) > 0 {
		cmd.Env = append(os.Environ(), "NODE_PATH="+strings.Join(nmDirs, string(os.PathListSeparator)))
	}

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

	// Index by displayName, preferring the entry with the most props
	// when there are duplicates (common with Radix re-exports).
	components := make(map[string]*DocgenResult, len(results))
	for i := range results {
		name := results[i].DisplayName
		existing, ok := components[name]
		if !ok || len(results[i].Props) > len(existing.Props) {
			components[name] = &results[i]
		}
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

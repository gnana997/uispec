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
	"github.com/gnana997/uispec/pkg/catalog"
)

//go:embed scripts/dist/storybook-worker.cjs
var storybookScript []byte

// DiscoverStoryFiles walks rootDir for .stories.{ts,tsx,js,jsx} files,
// applying exclude patterns (node_modules, dist, etc.).
func DiscoverStoryFiles(rootDir string, excludes []string) ([]string, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	var files []string
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == "node_modules" || base == ".git" || base == "dist" || base == "build" || base == ".next" {
				return filepath.SkipDir
			}
			return nil
		}

		// Must match *.stories.{ts,tsx,js,jsx}
		name := d.Name()
		if !isStoryFile(name) {
			return nil
		}

		// Check exclude patterns.
		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		for _, pattern := range excludes {
			if matched, _ := doublestar.Match(pattern, rel); matched {
				return nil
			}
		}

		files = append(files, path)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// isStoryFile checks if a filename matches the *.stories.{ts,tsx,js,jsx} pattern.
func isStoryFile(name string) bool {
	lowerParts := strings.ToLower(name)
	return strings.Contains(lowerParts, ".stories.") &&
		(strings.HasSuffix(lowerParts, ".ts") ||
			strings.HasSuffix(lowerParts, ".tsx") ||
			strings.HasSuffix(lowerParts, ".js") ||
			strings.HasSuffix(lowerParts, ".jsx"))
}

// RunStorybookExtraction executes the Storybook CSF worker to extract
// story examples from .stories.* files.
func RunStorybookExtraction(rootDir string, storyFiles []string, runtime string, log *slog.Logger) (*StorybookExtractionResult, error) {
	if log == nil {
		log = slog.Default()
	}

	if len(storyFiles) == 0 {
		return &StorybookExtractionResult{}, nil
	}

	start := time.Now()

	// Write the embedded script to a temp file with .cjs extension for CJS.
	tmpFile, err := os.CreateTemp("", "uispec-storybook-*.cjs")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.Write(storybookScript); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("failed to write storybook script: %w", err)
	}
	_ = tmpFile.Close()

	// Prepare input JSON.
	input := storybookInput{
		Files: storyFiles,
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

	log.Info("running storybook extraction",
		"runtime", filepath.Base(runtime),
		"storyFiles", len(storyFiles))

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			log.Warn("storybook worker stderr", "output", stderrStr)
		}
		return nil, fmt.Errorf("storybook worker failed: %w (stderr: %s)", err, stderrStr)
	}

	// Log warnings from stderr (non-fatal).
	if stderrStr := stderr.String(); stderrStr != "" {
		log.Debug("storybook worker warnings", "output", stderrStr)
	}

	// Parse output.
	var output storybookWorkerOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, fmt.Errorf("failed to parse storybook output: %w", err)
	}

	duration := time.Since(start).Milliseconds()

	totalStories := 0
	for _, r := range output.Results {
		totalStories += len(r.Stories)
	}

	log.Info("storybook extraction complete",
		"files", len(output.Results),
		"stories", totalStories,
		"runtime", filepath.Base(runtime),
		"ms", duration)

	return &StorybookExtractionResult{
		Results:    output.Results,
		Runtime:    filepath.Base(runtime),
		DurationMs: duration,
	}, nil
}

// BuildExamplesMap converts storybook extraction results into a map of
// component name → catalog examples, matching against detected components.
func BuildExamplesMap(sbResult *StorybookExtractionResult, components []DetectedComponent) map[string][]catalog.Example {
	if sbResult == nil || len(sbResult.Results) == 0 {
		return nil
	}

	// Build a set of known component names for matching.
	knownComponents := make(map[string]bool, len(components))
	for _, c := range components {
		knownComponents[c.Name] = true
	}

	examples := make(map[string][]catalog.Example)
	for _, sf := range sbResult.Results {
		compName := sf.ComponentName
		if !knownComponents[compName] {
			continue
		}

		for _, story := range sf.Stories {
			ex := catalog.Example{
				Title: story.Name,
				Code:  story.Code,
			}
			if story.HasPlayFunction {
				ex.Description = "Interactive story with play function"
			}
			examples[compName] = append(examples[compName], ex)
		}
	}

	return examples
}

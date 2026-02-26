package scanner

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/bmatcuk/doublestar/v4"
)

// DiscoverFiles walks rootDir applying include/exclude globs from cfg.
// Returns a sorted slice of absolute file paths for deterministic output.
func DiscoverFiles(rootDir string, cfg ScanConfig) ([]string, error) {
	// Validate patterns.
	for _, pattern := range cfg.Exclude {
		if !doublestar.ValidatePattern(pattern) {
			return nil, fmt.Errorf("invalid exclude pattern: %s", pattern)
		}
	}
	for _, pattern := range cfg.Include {
		if !doublestar.ValidatePattern(pattern) {
			return nil, fmt.Errorf("invalid include pattern: %s", pattern)
		}
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root path: %w", err)
	}

	var files []string

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Continue walking on errors.
		}

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)

		// Check exclusions (directories and files).
		for _, pattern := range cfg.Exclude {
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

		// Check include patterns.
		if len(cfg.Include) > 0 {
			matched := false
			for _, pattern := range cfg.Include {
				if m, _ := doublestar.PathMatch(pattern, relPath); m {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

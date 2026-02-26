package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/gnana997/uispec/catalogs"
	"github.com/gnana997/uispec/pkg/catalog"
)

// ProjectConfig holds the contents of .uispec/config.yaml.
type ProjectConfig struct {
	Version     string `yaml:"version"`
	Framework   string `yaml:"framework"`
	CatalogPath string `yaml:"catalog_path"`
}

// loadProjectConfig reads .uispec/config.yaml from the current directory.
// Returns nil (no error) if the file does not exist.
func loadProjectConfig() (*ProjectConfig, error) {
	data, err := os.ReadFile(".uispec/config.yaml")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// resolveCatalogPath returns the catalog path to use, applying the fallback chain:
//  1. Explicit --catalog flag value (non-empty override)
//  2. catalog_path from .uispec/config.yaml
//  3. Empty string — caller should use the embedded catalog bytes
func resolveCatalogPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if cfg, err := loadProjectConfig(); err == nil && cfg != nil && cfg.CatalogPath != "" {
		return cfg.CatalogPath
	}
	return ""
}

// loadCatalog loads a QueryService using the fallback chain:
//  1. If catalogPath is non-empty, load from that file (resolving relative to exe if needed)
//  2. Otherwise, load from the embedded bundled shadcn catalog (zero-config)
func loadCatalog(catalogPath string) (*catalog.QueryService, error) {
	if catalogPath == "" {
		return catalog.LoadAndQueryBytes(catalogs.ShadcnJSON)
	}

	// Resolve relative path against executable location as fallback.
	if !filepath.IsAbs(catalogPath) {
		if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
			exe, _ := os.Executable()
			alt := filepath.Join(filepath.Dir(exe), catalogPath)
			if _, err := os.Stat(alt); err == nil {
				catalogPath = alt
			}
		}
	}

	qs, err := catalog.LoadAndQuery(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load catalog: %w", err)
	}
	return qs, nil
}

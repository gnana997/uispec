package main

import (
	"os"

	"gopkg.in/yaml.v3"
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
//  3. Default: catalogs/shadcn/catalog.json
func resolveCatalogPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if cfg, err := loadProjectConfig(); err == nil && cfg != nil && cfg.CatalogPath != "" {
		return cfg.CatalogPath
	}
	return "catalogs/shadcn/catalog.json"
}

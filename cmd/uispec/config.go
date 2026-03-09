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
//  3. Single catalog in ~/.uispec/catalogs/ (auto-detected)
//  4. Empty string — caller should use the embedded catalog bytes
func resolveCatalogPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if cfg, err := loadProjectConfig(); err == nil && cfg != nil && cfg.CatalogPath != "" {
		return cfg.CatalogPath
	}
	if p := findUserCatalog(); p != "" {
		return p
	}
	return ""
}

// findUserCatalog checks ~/.uispec/catalogs/ for a single JSON catalog file.
// Returns its path if exactly one exists (unambiguous), empty string otherwise.
func findUserCatalog() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".uispec", "catalogs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var jsonFiles []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			jsonFiles = append(jsonFiles, filepath.Join(dir, e.Name()))
		}
	}

	if len(jsonFiles) == 1 {
		return jsonFiles[0]
	}
	return ""
}

// updateOrCreateConfig updates or creates .uispec/config.yaml with the given catalog path.
// If config.yaml already exists, only catalog_path is updated (other fields preserved).
func updateOrCreateConfig(catalogPath string) error {
	if err := os.MkdirAll(".uispec", 0755); err != nil {
		return fmt.Errorf("failed to create .uispec/: %w", err)
	}

	var cfg ProjectConfig
	existing, err := loadProjectConfig()
	if err == nil && existing != nil {
		cfg = *existing
	} else {
		cfg = ProjectConfig{
			Version:   "1",
			Framework: "react",
		}
	}
	cfg.CatalogPath = catalogPath

	body, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := "# UISpec project configuration\n# Updated by: uispec scan --init\n"
	return os.WriteFile(".uispec/config.yaml", append([]byte(header), body...), 0644)
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

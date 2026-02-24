package catalog

import (
	"encoding/json"
	"fmt"
	"os"
)

// Catalog holds the full design system specification.
type Catalog struct {
	Name       string      `json:"name"`
	Version    string      `json:"version"`
	Components []Component `json:"components"`
	Tokens     []Token     `json:"tokens"`
	Guidelines []Guideline `json:"guidelines"`
	Categories []Category  `json:"categories"`
}

// LoadFromFile loads a catalog from a JSON file.
func LoadFromFile(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog file: %w", err)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse catalog JSON: %w", err)
	}

	return &catalog, nil
}

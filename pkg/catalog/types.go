package catalog

// Component represents a UI component in the catalog.
type Component struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	ImportPath  string   `json:"import_path"`
	Props       []Prop   `json:"props"`
	Examples    []string `json:"examples,omitempty"`
}

// Prop represents a component property.
type Prop struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Required      bool     `json:"required"`
	Default       string   `json:"default,omitempty"`
	Description   string   `json:"description,omitempty"`
	AllowedValues []string `json:"allowed_values,omitempty"`
}

// Token represents a design token.
type Token struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Category string `json:"category"`
}

// Guideline represents a usage guideline or composition rule.
type Guideline struct {
	Rule        string `json:"rule"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // "error", "warning", "info"
}

// Category groups components logically.
type Category struct {
	Name       string `json:"name"`
	Components []string `json:"components"`
}

package catalog

// Component represents a top-level UI component in the catalog.
type Component struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Category      string         `json:"category"`
	ImportPath    string         `json:"import_path"`
	ImportedNames []string       `json:"imported_names"`
	Props         []Prop         `json:"props,omitempty"`
	SubComponents []SubComponent `json:"sub_components,omitempty"`
	Examples      []Example      `json:"examples,omitempty"`
	Guidelines    []Guideline    `json:"guidelines,omitempty"`
	Deprecated    bool           `json:"deprecated,omitempty"`
	DeprecatedMsg string         `json:"deprecated_msg,omitempty"`
}

// SubComponent represents a nested part of a compound component.
// For example, DialogTrigger and DialogContent are sub-components of Dialog.
type SubComponent struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Props           []Prop   `json:"props,omitempty"`
	MustContain     []string `json:"must_contain,omitempty"`
	AllowedChildren []string `json:"allowed_children,omitempty"`
	AllowedParents  []string `json:"allowed_parents,omitempty"`
}

// Prop represents a component property.
type Prop struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Required      bool     `json:"required"`
	Default       string   `json:"default,omitempty"`
	Description   string   `json:"description,omitempty"`
	AllowedValues []string `json:"allowed_values,omitempty"`
	Deprecated    bool     `json:"deprecated,omitempty"`
}

// Example represents a usage example for a component.
type Example struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Code        string `json:"code"`
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
	Severity    string `json:"severity"`           // "error", "warning", "info"
	Component   string `json:"component,omitempty"` // optional: scopes to a specific component
}

// Category groups components logically.
type Category struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Components  []string `json:"components"`
}

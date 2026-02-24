package validator

// Validator checks source code against the design system catalog.
type Validator struct {
	// TODO: catalog, parser manager, query manager references
}

// ValidationResult represents the result of validating a file.
type ValidationResult struct {
	FilePath   string
	Valid      bool
	Violations []Violation
}

// Violation represents a single validation rule violation.
type Violation struct {
	Rule       string
	Message    string
	Severity   string // "error", "warning", "info"
	Line       int
	Column     int
	Suggestion string
}

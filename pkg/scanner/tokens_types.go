package scanner

// tokenInput is the JSON sent to the tokens worker via stdin.
type tokenInput struct {
	CSSFiles []string `json:"cssFiles"`
}

// tokenWorkerOutput is the JSON received from the tokens worker via stdout.
type tokenWorkerOutput struct {
	Tokens   []tokenResult `json:"tokens"`
	DarkMode bool          `json:"darkMode"`
}

// tokenResult represents a single design token from the worker output.
type tokenResult struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Category string `json:"category"`
}

// TokenExtractionResult holds the output of the token extraction phase.
type TokenExtractionResult struct {
	Tokens     []tokenResult
	DarkMode   bool
	Runtime    string
	DurationMs int64
}

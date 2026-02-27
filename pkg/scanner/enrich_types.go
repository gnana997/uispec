package scanner

// DocgenResult represents a single component's output from react-docgen-typescript.
// This mirrors the JSON produced by scripts/docgen-worker.ts.
type DocgenResult struct {
	DisplayName string       `json:"displayName"`
	FilePath    string       `json:"filePath"`
	Description string       `json:"description"`
	Props       []DocgenProp `json:"props"`
}

// DocgenProp represents a single prop from react-docgen-typescript output.
type DocgenProp struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Required      bool     `json:"required"`
	DefaultValue  string   `json:"defaultValue"`
	Description   string   `json:"description"`
	Deprecated    bool     `json:"deprecated"`
	AllowedValues []string `json:"allowedValues"`
	Parent        string   `json:"parent"`
}

// docgenInput is the JSON sent to the docgen worker via stdin.
type docgenInput struct {
	Files    []string `json:"files"`
	TSConfig string   `json:"tsconfig"`
}

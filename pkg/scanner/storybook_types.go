package scanner

// storybookInput is the JSON sent to the storybook worker via stdin.
type storybookInput struct {
	Files []string `json:"files"`
}

// storybookWorkerOutput is the JSON received from the storybook worker via stdout.
type storybookWorkerOutput struct {
	Results []storyFileResult `json:"results"`
}

// storyFileResult represents the parsed stories from a single .stories.* file.
type storyFileResult struct {
	FilePath        string      `json:"filePath"`
	ComponentName   string      `json:"componentName"`
	ComponentImport string      `json:"componentImport"`
	Title           string      `json:"title"`
	Description     string      `json:"description"`
	Stories         []storyInfo `json:"stories"`
}

// storyInfo represents a single story extracted from a CSF file.
type storyInfo struct {
	Name              string `json:"name"`
	ExportName        string `json:"exportName"`
	Code              string `json:"code"`
	HasPlayFunction   bool   `json:"hasPlayFunction"`
	HasRenderFunction bool   `json:"hasRenderFunction"`
}

// StorybookExtractionResult holds the output of the storybook extraction phase.
type StorybookExtractionResult struct {
	Results    []storyFileResult
	Runtime    string
	DurationMs int64
}

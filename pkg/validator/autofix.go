package validator

// AutoFix represents a deterministic code fix that can be applied without LLM involvement.
type AutoFix struct {
	Line    int
	OldText string
	NewText string
	Reason  string
}

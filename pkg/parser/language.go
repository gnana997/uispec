package parser

import (
	"path/filepath"
	"strings"
)

// Language represents a supported programming language for parsing.
type Language int

const (
	// LanguageTypeScript represents TypeScript (.ts, .tsx files)
	LanguageTypeScript Language = iota
	// LanguageJavaScript represents JavaScript (.js, .jsx files)
	LanguageJavaScript
	// LanguageUnknown represents an unsupported language
	LanguageUnknown
)

// String returns the string representation of the language.
func (l Language) String() string {
	switch l {
	case LanguageTypeScript:
		return "typescript"
	case LanguageJavaScript:
		return "javascript"
	default:
		return "unknown"
	}
}

// DetectLanguage detects the programming language from a file path.
// Returns LanguageUnknown if the file extension is not recognized.
func DetectLanguage(filePath string) Language {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".ts", ".mts", ".cts":
		return LanguageTypeScript
	case ".tsx":
		return LanguageTypeScript // TSX is handled separately via IsTSXFile
	case ".js", ".jsx", ".mjs", ".cjs":
		return LanguageJavaScript
	default:
		return LanguageUnknown
	}
}

// IsTSXFile checks if a file path represents a TSX file.
// TSX files use the TypeScript grammar with JSX support enabled.
func IsTSXFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return ext == ".tsx"
}

// IsJSXFile checks if a file path represents a JSX file.
// JSX files use the JavaScript grammar.
func IsJSXFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return ext == ".jsx"
}

// ParseLanguageString converts a language string to a Language type.
// Returns LanguageUnknown if the string is not recognized.
func ParseLanguageString(lang string) Language {
	switch strings.ToLower(lang) {
	case "typescript", "ts":
		return LanguageTypeScript
	case "javascript", "js":
		return LanguageJavaScript
	default:
		return LanguageUnknown
	}
}

// SupportedLanguages returns a list of all supported languages.
func SupportedLanguages() []Language {
	return []Language{
		LanguageTypeScript,
		LanguageJavaScript,
	}
}

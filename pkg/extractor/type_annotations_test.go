package extractor

import (
	"log/slog"
	"testing"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
)

// TestTypeScriptTypeAnnotations tests type annotation extraction from TypeScript code.
func TestTypeScriptTypeAnnotations(t *testing.T) {
	// Create parser and query managers
	pm := parser.NewParserManager(slog.Default())
	defer pm.Close()

	qm := queries.NewQueryManager(pm, slog.Default())
	defer qm.Close()

	// Create extractor
	extractor := NewExtractor(pm, qm, slog.Default())

	tests := []struct {
		name     string
		code     string
		expected map[string]string // varName → typeName
	}{
		{
			name: "Variable with simple type",
			code: `const service: UserService = new UserService();`,
			expected: map[string]string{
				"service": "UserService",
			},
		},
		{
			name: "Variable with generic type - extract first type argument",
			code: `const users: Array<User> = [];`,
			expected: map[string]string{
				"users": "User", // Should extract first type argument
			},
		},
		{
			name: "Function parameter with type",
			code: `function process(data: DataType) { }`,
			expected: map[string]string{
				"data": "DataType",
			},
		},
		{
			name: "Class property with type",
			code: `class Service {
				private api: ApiClient;
			}`,
			expected: map[string]string{
				"api": "ApiClient",
			},
		},
		{
			name: "Multiple variables with types",
			code: `
				const service: UserService = new UserService();
				const api: AxiosInstance = axios.create();
				let count: number = 0;
			`,
			expected: map[string]string{
				"service": "UserService",
				"api":     "AxiosInstance",
				"count":   "number",
			},
		},
		{
			name: "Arrow function parameter with type",
			code: `const handler = (req: Request, res: Response) => { };`,
			expected: map[string]string{
				"req": "Request",
				"res": "Response",
			},
		},
		{
			name: "Generic with multiple type arguments - extracts first",
			code: `const map: Map<string, number> = new Map();`,
			expected: map[string]string{
				// With + quantifier, should extract first type argument
				"map": "string", // Key type (first arg) is most useful for resolution
			},
		},
		{
			name: "Type assertion with as keyword",
			code: `const service = obj as UserService;`,
			expected: map[string]string{
				"service": "UserService",
			},
		},
		{
			name: "Object destructuring with type (not supported yet)",
			code: `const { user }: { user: User } = getData();`,
			expected: map[string]string{
				// Not expected to work in Phase 2 - requires full inference
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract from file
			result, err := extractor.ExtractFile("test.ts", []byte(tt.code))
			if err != nil {
				t.Fatalf("Failed to extract: %v", err)
			}

			// Verify type annotations
			if len(result.TypeAnnotations) != len(tt.expected) {
				t.Errorf("Expected %d type annotations, got %d", len(tt.expected), len(result.TypeAnnotations))
				t.Logf("Expected: %v", tt.expected)
				t.Logf("Got: %v", result.TypeAnnotations)
			}

			for varName, expectedType := range tt.expected {
				actualType, found := result.TypeAnnotations[varName]
				if !found {
					t.Errorf("Expected type annotation for %q, but not found", varName)
					continue
				}
				if actualType != expectedType {
					t.Errorf("For variable %q: expected type %q, got %q", varName, expectedType, actualType)
				}
			}

			// Log all extracted annotations for debugging
			if len(result.TypeAnnotations) > 0 {
				t.Logf("Extracted type annotations: %v", result.TypeAnnotations)
			}
		})
	}
}

// TestJavaScriptTypeAnnotations tests that JavaScript files also extract types (from JSDoc or similar).
func TestJavaScriptTypeAnnotations(t *testing.T) {
	// Create parser and query managers
	pm := parser.NewParserManager(slog.Default())
	defer pm.Close()

	qm := queries.NewQueryManager(pm, slog.Default())
	defer qm.Close()

	// Create extractor
	extractor := NewExtractor(pm, qm, slog.Default())

	tests := []struct {
		name     string
		code     string
		expected map[string]string
	}{
		{
			name: "JavaScript with TypeScript-style type annotations (if supported)",
			code: `const service: UserService = new UserService();`,
			expected: map[string]string{
				"service": "UserService",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract from JavaScript file
			result, err := extractor.ExtractFile("test.js", []byte(tt.code))
			if err != nil {
				t.Fatalf("Failed to extract: %v", err)
			}

			// Verify type annotations
			for varName, expectedType := range tt.expected {
				actualType, found := result.TypeAnnotations[varName]
				if !found {
					// JavaScript type annotations are optional - skip if not found
					t.Logf("Type annotation for %q not found (expected in JS+TS mode)", varName)
					continue
				}
				if actualType != expectedType {
					t.Errorf("For variable %q: expected type %q, got %q", varName, expectedType, actualType)
				}
			}

			t.Logf("Extracted type annotations from JS: %v", result.TypeAnnotations)
		})
	}
}

// TestTypeAnnotationsRealWorldExample tests type extraction from realistic code patterns.
func TestTypeAnnotationsRealWorldExample(t *testing.T) {
	// Create parser and query managers
	pm := parser.NewParserManager(slog.Default())
	defer pm.Close()

	qm := queries.NewQueryManager(pm, slog.Default())
	defer qm.Close()

	// Create extractor
	extractor := NewExtractor(pm, qm, slog.Default())

	code := `
		import axios, { AxiosInstance } from 'axios';
		import { UserService } from './services/UserService';

		// Create API client
		const api: AxiosInstance = axios.create({
			baseURL: 'https://api.example.com'
		});

		// Create service
		const userService: UserService = new UserService(api);

		// Use service
		async function getUser(id: number): Promise<User> {
			const user: User = await userService.getUser(id);
			return user;
		}

		// HTTP handler
		export function handler(req: Request, res: Response): void {
			const userId: string = req.params.id;
			// ...
		}
	`

	result, err := extractor.ExtractFile("app.ts", []byte(code))
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	// Expected type annotations
	expected := map[string]string{
		"api":         "AxiosInstance",
		"userService": "UserService",
		"user":        "User",
		"id":          "number", // Function parameter
		"req":         "Request",
		"res":         "Response",
		"userId":      "string",
	}

	// Verify each expected annotation
	for varName, expectedType := range expected {
		actualType, found := result.TypeAnnotations[varName]
		if !found {
			t.Errorf("Expected type annotation for %q, but not found", varName)
			continue
		}
		if actualType != expectedType {
			t.Errorf("For variable %q: expected type %q, got %q", varName, expectedType, actualType)
		}
	}

	// Log results
	t.Logf("Extracted %d type annotations from real-world code", len(result.TypeAnnotations))
	t.Logf("Type annotations: %v", result.TypeAnnotations)

	// Verify we also extracted symbols, imports, etc.
	t.Logf("Extracted %d symbols", len(result.Symbols))
	t.Logf("Extracted %d imports", len(result.Imports))
	t.Logf("Extracted %d exports", len(result.Exports))
}

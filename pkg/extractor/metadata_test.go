// Metadata extraction tests.
package extractor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
)

// setupExtractorForMetadataTests creates an extractor for metadata testing
func setupExtractorForMetadataTests(t *testing.T) *Extractor {
	pm := parser.NewParserManager(nil)
	qm := queries.NewQueryManager(pm, nil)
	return NewExtractor(pm, qm, nil)
}

// ============================================================================
// TypeScript/JavaScript Metadata Tests
// ============================================================================

func TestExtractTSMetadata_Visibility(t *testing.T) {
	extractor := setupExtractorForMetadataTests(t)

	code := `
class UserService {
  public getName() { return "name"; }
  private getAge() { return 42; }
  protected getEmail() { return "email"; }
}
`

	result, err := extractor.ExtractFile("test.ts", []byte(code))
	require.NoError(t, err)
	require.NotNil(t, result)

	symbolMap := make(map[string]Symbol)
	for _, sym := range result.Symbols {
		symbolMap[sym.Name] = sym
	}

	// Check public method
	if getName, found := symbolMap["getName"]; found {
		assert.Equal(t, "public", getName.Scope, "getName should be public")
	}

	// Check private method
	if getAge, found := symbolMap["getAge"]; found {
		assert.Equal(t, "private", getAge.Scope, "getAge should be private")
	}

	// Check protected method
	if getEmail, found := symbolMap["getEmail"]; found {
		assert.Equal(t, "protected", getEmail.Scope, "getEmail should be protected")
	}
}

func TestExtractTSMetadata_Modifiers(t *testing.T) {
	extractor := setupExtractorForMetadataTests(t)

	code := `
class DataService {
  static async fetchData() { return []; }
  readonly maxRetries = 3;
  abstract validate();
}
`

	result, err := extractor.ExtractFile("test.ts", []byte(code))
	require.NoError(t, err)
	require.NotNil(t, result)

	symbolMap := make(map[string]Symbol)
	for _, sym := range result.Symbols {
		symbolMap[sym.Name] = sym
	}

	// Check static and async modifiers
	if fetchData, found := symbolMap["fetchData"]; found {
		assert.Contains(t, fetchData.Modifiers, "static", "fetchData should have static modifier")
		assert.Contains(t, fetchData.Modifiers, "async", "fetchData should have async modifier")
	}

	// Check readonly modifier
	if maxRetries, found := symbolMap["maxRetries"]; found {
		assert.Contains(t, maxRetries.Modifiers, "readonly", "maxRetries should have readonly modifier")
	}

	// Check abstract modifier
	if validate, found := symbolMap["validate"]; found {
		assert.Contains(t, validate.Modifiers, "abstract", "validate should have abstract modifier")
	}
}

func TestExtractTSMetadata_Parameters(t *testing.T) {
	extractor := setupExtractorForMetadataTests(t)

	code := `
function processUser(name: string, age: number, active?: boolean) {
  return { name, age, active };
}
`

	result, err := extractor.ExtractFile("test.ts", []byte(code))
	require.NoError(t, err)
	require.NotNil(t, result)

	symbolMap := make(map[string]Symbol)
	for _, sym := range result.Symbols {
		symbolMap[sym.Name] = sym
	}

	if processUser, found := symbolMap["processUser"]; found {
		assert.Len(t, processUser.Parameters, 3, "processUser should have 3 parameters")
		assert.Equal(t, "name", processUser.Parameters[0])
		assert.Equal(t, "age", processUser.Parameters[1])
		assert.Equal(t, "active", processUser.Parameters[2])

		// Check parameter types
		assert.Len(t, processUser.ParameterTypes, 3, "Should have 3 parameter types")
		assert.Equal(t, "string", processUser.ParameterTypes[0])
		assert.Equal(t, "number", processUser.ParameterTypes[1])
		assert.Equal(t, "boolean", processUser.ParameterTypes[2])
	}
}

func TestExtractTSMetadata_ReturnType(t *testing.T) {
	extractor := setupExtractorForMetadataTests(t)

	code := `
function getCount(): number {
  return 42;
}

async function fetchUsers(): Promise<User[]> {
  return [];
}
`

	result, err := extractor.ExtractFile("test.ts", []byte(code))
	require.NoError(t, err)
	require.NotNil(t, result)

	symbolMap := make(map[string]Symbol)
	for _, sym := range result.Symbols {
		symbolMap[sym.Name] = sym
	}

	if getCount, found := symbolMap["getCount"]; found {
		assert.Equal(t, "number", getCount.ReturnType, "getCount should return number")
	}

	if fetchUsers, found := symbolMap["fetchUsers"]; found {
		assert.Equal(t, "Promise<User[]>", fetchUsers.ReturnType, "fetchUsers should return Promise<User[]>")
	}
}

func TestExtractTSMetadata_MethodsWithTypes(t *testing.T) {
	extractor := setupExtractorForMetadataTests(t)

	code := `
class Calculator {
  add(a: number, b: number): number {
    return a + b;
  }

  async multiply(x: number, y: number): Promise<number> {
    return x * y;
  }
}
`

	result, err := extractor.ExtractFile("test.ts", []byte(code))
	require.NoError(t, err)
	require.NotNil(t, result)

	symbolMap := make(map[string]Symbol)
	for _, sym := range result.Symbols {
		symbolMap[sym.Name] = sym
	}

	// Check add method
	if add, found := symbolMap["add"]; found {
		assert.Len(t, add.Parameters, 2)
		assert.Equal(t, "a", add.Parameters[0])
		assert.Equal(t, "b", add.Parameters[1])
		assert.Len(t, add.ParameterTypes, 2)
		assert.Equal(t, "number", add.ParameterTypes[0])
		assert.Equal(t, "number", add.ParameterTypes[1])
		assert.Equal(t, "number", add.ReturnType)
	}

	// Check multiply method
	if multiply, found := symbolMap["multiply"]; found {
		assert.Contains(t, multiply.Modifiers, "async")
		assert.Len(t, multiply.Parameters, 2)
		assert.Equal(t, "Promise<number>", multiply.ReturnType)
	}
}

// ============================================================================
// Integration Tests (using real testdata files)
// ============================================================================

func TestExtractMetadata_RealFiles(t *testing.T) {
	extractor := setupExtractorForMetadataTests(t)

	testFiles := []struct {
		file     string
		language parser.Language
		checks   func(*testing.T, []Symbol)
	}{
		{
			file:     filepath.Join("testdata", "sample.ts"),
			language: parser.LanguageTypeScript,
			checks: func(t *testing.T, symbols []Symbol) {
				symbolMap := make(map[string]Symbol)
				for _, sym := range symbols {
					symbolMap[sym.Name] = sym
				}

				// Check that UserService class methods have metadata
				if getUser, found := symbolMap["getUser"]; found {
					assert.NotEmpty(t, getUser.Parameters, "getUser should have parameters")
					assert.NotEmpty(t, getUser.ReturnType, "getUser should have return type")
				}
			},
		},
	}

	for _, tc := range testFiles {
		t.Run(tc.file, func(t *testing.T) {
			sourceCode, err := os.ReadFile(tc.file)
			require.NoError(t, err, "Failed to read test file")

			result, err := extractor.ExtractFile(tc.file, sourceCode)
			require.NoError(t, err, "Failed to extract from test file")
			require.NotNil(t, result, "Result should not be nil")

			tc.checks(t, result.Symbols)
		})
	}
}


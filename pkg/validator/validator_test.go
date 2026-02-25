package validator

import (
	"testing"

	"github.com/gnana997/uispec/pkg/catalog"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testValidator() *Validator {
	cat := &catalog.Catalog{
		Name:    "test",
		Version: "1.0",
		Categories: []catalog.Category{
			{Name: "actions", Components: []string{"Button"}},
			{Name: "overlay", Components: []string{"Dialog"}},
		},
		Components: []catalog.Component{
			{
				Name:          "Button",
				Description:   "A button",
				Category:      "actions",
				ImportPath:    "@/components/ui/button",
				ImportedNames: []string{"Button"},
				Props: []catalog.Prop{
					{Name: "variant", Type: "string", AllowedValues: []string{"default", "destructive", "outline"}, Default: "default"},
					{Name: "size", Type: "string", AllowedValues: []string{"default", "sm", "lg"}},
					{Name: "asChild", Type: "boolean"},
				},
			},
			{
				Name:          "Dialog",
				Description:   "A dialog",
				Category:      "overlay",
				ImportPath:    "@/components/ui/dialog",
				ImportedNames: []string{"Dialog", "DialogTrigger", "DialogContent", "DialogTitle"},
				SubComponents: []catalog.SubComponent{
					{Name: "DialogTrigger", Description: "Opens the dialog", AllowedParents: []string{"Dialog"}},
					{Name: "DialogContent", Description: "Content container", AllowedParents: []string{"Dialog"}, MustContain: []string{"DialogTitle"}},
					{Name: "DialogTitle", Description: "Title", AllowedParents: []string{"DialogContent"}},
				},
			},
		},
	}
	idx := cat.BuildIndex()
	pm := parser.NewParserManager(nil)
	return NewValidator(cat, idx, pm)
}

func TestValidatePage_ValidCode(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `
import { Button } from "@/components/ui/button"

export default function Page() {
  return <Button variant="default">Click</Button>
}
`
	result := v.ValidatePage(code, false)
	assert.True(t, result.Valid)
	assert.Equal(t, "no issues found", result.Summary)
}

func TestValidatePage_UnknownComponent(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `export default function Page() { return <FancyWidget /> }`
	result := v.ValidatePage(code, false)

	require.NotEmpty(t, result.Violations)
	assert.Equal(t, "unknown-component", result.Violations[0].Rule)
	assert.Equal(t, "FancyWidget", result.Violations[0].Component)
}

func TestValidatePage_MissingImport(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `export default function Page() { return <Button>Click</Button> }`
	result := v.ValidatePage(code, false)

	var found bool
	for _, viol := range result.Violations {
		if viol.Rule == "missing-import" && viol.Component == "Button" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected missing-import violation for Button")
}

func TestValidatePage_WrongImportPath(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `
import { Button } from "wrong/path"

export default function Page() { return <Button>Click</Button> }
`
	result := v.ValidatePage(code, false)

	var found bool
	for _, viol := range result.Violations {
		if viol.Rule == "wrong-import-path" && viol.Component == "Button" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected wrong-import-path violation for Button")
}

func TestValidatePage_InvalidPropValue(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `
import { Button } from "@/components/ui/button"

export default function Page() {
  return <Button variant="fancy">Click</Button>
}
`
	result := v.ValidatePage(code, false)

	var found bool
	for _, viol := range result.Violations {
		if viol.Rule == "invalid-prop-value" {
			found = true
			assert.Contains(t, viol.Message, "fancy")
			break
		}
	}
	assert.True(t, found, "expected invalid-prop-value violation")
}

func TestValidatePage_CompositionViolation(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	// DialogContent outside of Dialog.
	code := `
import { DialogContent, DialogTitle } from "@/components/ui/dialog"

export default function Page() {
  return <DialogContent><DialogTitle>Hi</DialogTitle></DialogContent>
}
`
	result := v.ValidatePage(code, false)

	var found bool
	for _, viol := range result.Violations {
		if viol.Rule == "composition-violation" && viol.Component == "DialogContent" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected composition-violation for DialogContent outside Dialog")
}

func TestValidatePage_MissingChild(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	// DialogContent without DialogTitle.
	code := `
import { Dialog, DialogContent } from "@/components/ui/dialog"

export default function Page() {
  return (
    <Dialog>
      <DialogContent>
        <p>No title here</p>
      </DialogContent>
    </Dialog>
  )
}
`
	result := v.ValidatePage(code, false)

	var found bool
	for _, viol := range result.Violations {
		if viol.Rule == "missing-child" && viol.Component == "DialogContent" {
			found = true
			assert.Contains(t, viol.Message, "DialogTitle")
			break
		}
	}
	assert.True(t, found, "expected missing-child violation for DialogContent missing DialogTitle")
}

func TestValidatePage_ValidCompoundComponent(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `
import { Dialog, DialogContent, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"

export default function Page() {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline">Open</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogTitle>Hello</DialogTitle>
      </DialogContent>
    </Dialog>
  )
}
`
	result := v.ValidatePage(code, false)

	// Filter to only errors (warnings for sub-component imports are ok).
	errors := filterBySeverity(result.Violations, "error")
	assert.Empty(t, errors, "valid compound component should have no errors, got: %+v", errors)
}

func TestValidatePage_AutoFixMissingImport(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `export default function Page() { return <Button>Click</Button> }`
	result := v.ValidatePage(code, true)

	require.NotEmpty(t, result.Fixes)
	assert.NotEmpty(t, result.FixedCode)
	assert.Contains(t, result.FixedCode, `import { Button } from "@/components/ui/button"`)
}

func TestValidatePage_AutoFixInvalidPropValue(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `
import { Button } from "@/components/ui/button"

export default function Page() {
  return <Button variant="fancy">Click</Button>
}
`
	result := v.ValidatePage(code, true)

	var propFix *AutoFix
	for i := range result.Fixes {
		if result.Fixes[i].Rule == "invalid-prop-value" {
			propFix = &result.Fixes[i]
			break
		}
	}
	require.NotNil(t, propFix, "expected a prop value fix")
	assert.Contains(t, result.FixedCode, `variant="default"`)
}

func TestValidatePage_Summary(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `export default function Page() { return <Button variant="fancy">Click</Button> }`
	result := v.ValidatePage(code, false)

	assert.NotEmpty(t, result.Summary)
	assert.NotEqual(t, "no issues found", result.Summary)
}

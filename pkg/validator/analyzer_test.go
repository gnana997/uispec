package validator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzePage_BasicStructure(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"

export default function Page() {
  return (
    <div>
      <Button variant="default" size="lg">Click</Button>
      <Dialog>
        <DialogContent>
          <DialogTitle>Hello</DialogTitle>
        </DialogContent>
      </Dialog>
    </div>
  )
}
`
	analysis := v.AnalyzePage(code)

	// Should have 4 components: Button, Dialog, DialogContent, DialogTitle.
	require.Len(t, analysis.Components, 4)

	assert.Equal(t, "Button", analysis.Components[0].Name)
	assert.Contains(t, analysis.Components[0].Props, "variant")
	assert.Contains(t, analysis.Components[0].Props, "size")

	assert.Equal(t, "Dialog", analysis.Components[1].Name)

	// Imports.
	require.Len(t, analysis.Imports, 2)
	assert.Equal(t, "@/components/ui/button", analysis.Imports[0])
	assert.Equal(t, "@/components/ui/dialog", analysis.Imports[1])

	// Line count.
	assert.Greater(t, analysis.LineCount, 10)
}

func TestAnalyzePage_EmptyCode(t *testing.T) {
	v := testValidator()
	defer v.parser.Close()

	code := `export default function Page() { return null }`
	analysis := v.AnalyzePage(code)

	assert.Empty(t, analysis.Components)
	assert.Equal(t, 1, analysis.LineCount)
}

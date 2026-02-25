package validator

import (
	"testing"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseTSX(t *testing.T, code string) *JSXExtraction {
	t.Helper()
	pm := parser.NewParserManager(nil)
	defer pm.Close()

	tree, err := pm.Parse([]byte(code), parser.LanguageTypeScript, true)
	require.NoError(t, err)
	defer tree.Close()

	return ExtractJSX(tree, []byte(code))
}

func TestExtractJSX_SimpleComponent(t *testing.T) {
	code := `<Button variant="default">Click</Button>`
	ext := parseTSX(t, code)

	require.Len(t, ext.Usages, 1)
	assert.Equal(t, "Button", ext.Usages[0].ComponentName)
	assert.Equal(t, "default", ext.Usages[0].Props["variant"])
	assert.True(t, ext.Usages[0].HasChildren)
	assert.Equal(t, "", ext.Usages[0].ParentComponent)
}

func TestExtractJSX_SelfClosing(t *testing.T) {
	code := `<Input placeholder="text" />`
	ext := parseTSX(t, code)

	require.Len(t, ext.Usages, 1)
	assert.Equal(t, "Input", ext.Usages[0].ComponentName)
	assert.Equal(t, "text", ext.Usages[0].Props["placeholder"])
	assert.False(t, ext.Usages[0].HasChildren)
}

func TestExtractJSX_NestedCompound(t *testing.T) {
	code := `
<Dialog>
  <DialogContent>
    <DialogTitle>Hello</DialogTitle>
  </DialogContent>
</Dialog>
`
	ext := parseTSX(t, code)

	require.Len(t, ext.Usages, 3)

	// Dialog
	assert.Equal(t, "Dialog", ext.Usages[0].ComponentName)
	assert.Equal(t, "", ext.Usages[0].ParentComponent)

	// DialogContent
	assert.Equal(t, "DialogContent", ext.Usages[1].ComponentName)
	assert.Equal(t, "Dialog", ext.Usages[1].ParentComponent)

	// DialogTitle
	assert.Equal(t, "DialogTitle", ext.Usages[2].ComponentName)
	assert.Equal(t, "DialogContent", ext.Usages[2].ParentComponent)
}

func TestExtractJSX_HTMLElementsSkipped(t *testing.T) {
	code := `
<div>
  <Button>Click</Button>
  <span>text</span>
</div>
`
	ext := parseTSX(t, code)

	// Only Button is a component (div and span are HTML elements).
	require.Len(t, ext.Usages, 1)
	assert.Equal(t, "Button", ext.Usages[0].ComponentName)
	// Parent is not tracked for HTML elements — no component parent.
	assert.Equal(t, "", ext.Usages[0].ParentComponent)
}

func TestExtractJSX_ParentThroughHTML(t *testing.T) {
	code := `
<Dialog>
  <div>
    <DialogContent>Hi</DialogContent>
  </div>
</Dialog>
`
	ext := parseTSX(t, code)

	require.Len(t, ext.Usages, 2)
	// DialogContent's parent is Dialog (HTML div is transparent).
	assert.Equal(t, "DialogContent", ext.Usages[1].ComponentName)
	assert.Equal(t, "Dialog", ext.Usages[1].ParentComponent)
}

func TestExtractJSX_ExpressionProps(t *testing.T) {
	code := `<Button onClick={() => {}} disabled={isLoading}>Click</Button>`
	ext := parseTSX(t, code)

	require.Len(t, ext.Usages, 1)
	props := ext.Usages[0].Props
	assert.Equal(t, "", props["onClick"]) // expression → empty string
	assert.Equal(t, "", props["disabled"])
}

func TestExtractJSX_BooleanProp(t *testing.T) {
	code := `<DialogTrigger asChild><Button>Open</Button></DialogTrigger>`
	ext := parseTSX(t, code)

	require.GreaterOrEqual(t, len(ext.Usages), 1)
	assert.Equal(t, "true", ext.Usages[0].Props["asChild"])
}

func TestExtractJSX_ImportExtraction(t *testing.T) {
	code := `
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent } from "@/components/ui/dialog"
import React from "react"

export function Page() {
  return <Button>Click</Button>
}
`
	ext := parseTSX(t, code)

	require.Len(t, ext.Imports, 3)

	// Button import.
	assert.Equal(t, "@/components/ui/button", ext.Imports[0].Source)
	assert.Equal(t, []string{"Button"}, ext.Imports[0].Names)

	// Dialog import.
	assert.Equal(t, "@/components/ui/dialog", ext.Imports[1].Source)
	assert.Equal(t, []string{"Dialog", "DialogContent"}, ext.Imports[1].Names)

	// React default import.
	assert.Equal(t, "react", ext.Imports[2].Source)
	assert.Equal(t, "React", ext.Imports[2].DefaultName)
}

func TestExtractJSX_EmptyCode(t *testing.T) {
	code := `export default function Page() { return null }`
	ext := parseTSX(t, code)

	assert.Empty(t, ext.Usages)
}

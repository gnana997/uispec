# UISpec Catalog Format

> Back to [main README](../README.md)

UISpec reads a single `catalog.json` file that describes a UI component library. This document covers the schema so you can author or generate catalogs for any design system.

## Quick start

```bash
# Use the bundled shadcn catalog (zero-config)
uispec serve

# Point at a custom catalog
uispec serve --catalog path/to/my-catalog.json
uispec validate src/page.tsx --catalog path/to/my-catalog.json

# Verify your catalog loads and inspect a component
uispec inspect Button --catalog path/to/my-catalog.json
```

## Top-level structure

```json
{
  "name": "my-design-system",
  "version": "1.0",
  "framework": "react",
  "source": "https://github.com/org/design-system",
  "categories": [],
  "components": [],
  "tokens": [],
  "guidelines": []
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Name of the design system |
| `version` | string | yes | Catalog schema version |
| `framework` | string | no | Target framework (e.g. `react`, `vue`) |
| `source` | string | no | URL or identifier for the source library |
| `categories` | Category[] | yes | Groupings for components |
| `components` | Component[] | yes | Full component definitions |
| `tokens` | Token[] | no | Design tokens (colors, spacing, etc.) |
| `guidelines` | Guideline[] | no | Global guidelines and rules |

## Categories

Categories group components for browsing. Every component references a category by name.

```json
{
  "name": "forms",
  "description": "Form input components",
  "components": ["Input", "Select", "Checkbox"]
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Unique category identifier |
| `description` | string | no | Human-readable description |
| `components` | string[] | yes | Component names in this category |

## Components

A component is the core unit. It describes what the component does, how to import it, what props it accepts, and how it composes with other components.

```json
{
  "name": "Button",
  "description": "Displays a button or a component that looks like a button.",
  "category": "actions",
  "import_path": "@/components/ui/button",
  "imported_names": ["Button"],
  "props": [],
  "sub_components": [],
  "examples": [],
  "guidelines": [],
  "deprecated": false,
  "deprecated_msg": ""
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Component name (PascalCase, must be unique) |
| `description` | string | no | What the component does |
| `category` | string | no | Must reference a defined category name |
| `import_path` | string | yes | Module path for imports (e.g. `@/components/ui/button`) |
| `imported_names` | string[] | yes | Named exports to import (at least one) |
| `props` | Prop[] | no | Props the component accepts |
| `sub_components` | SubComponent[] | no | Compound component parts (e.g. DialogContent) |
| `examples` | Example[] | no | Code examples |
| `guidelines` | Guideline[] | no | Component-scoped rules |
| `deprecated` | boolean | no | Mark as deprecated |
| `deprecated_msg` | string | no | Migration guidance for deprecated components |

### What the validator checks

- **`import_path`** + **`imported_names`** — validates that imports in code match the catalog
- **`props[].allowed_values`** — validates that prop values are in the enum
- **`props[].required`** — detects missing required props
- **`sub_components[].allowed_parents`** — validates composition (e.g. `CardContent` must be inside `Card`)
- **`sub_components[].must_contain`** — validates that parent contains required children
- **`deprecated`** — flags usage of deprecated components

## Props

```json
{
  "name": "variant",
  "type": "string",
  "required": false,
  "default": "default",
  "description": "Visual style variant",
  "allowed_values": ["default", "destructive", "outline", "ghost"],
  "deprecated": false
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Prop name |
| `type` | string | yes | Type (`string`, `boolean`, `number`, `ReactNode`, etc.) |
| `required` | boolean | yes | Whether the prop must be provided |
| `default` | string | no | Default value if not provided |
| `description` | string | no | What the prop does |
| `allowed_values` | string[] | no | Enum of valid values — the validator rejects anything not in this list |
| `deprecated` | boolean | no | Mark prop as deprecated |

## Sub-components

For compound components (Dialog, Alert, Table, etc.), sub-components describe the parts and their composition rules.

```json
{
  "name": "AlertDialogContent",
  "description": "Contains the dialog content, title, and actions.",
  "props": [],
  "must_contain": ["AlertDialogTitle"],
  "allowed_children": ["AlertDialogHeader", "AlertDialogFooter", "AlertDialogTitle", "AlertDialogDescription", "AlertDialogAction", "AlertDialogCancel"],
  "allowed_parents": ["AlertDialog"]
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Sub-component name (PascalCase) |
| `description` | string | no | What this part does |
| `props` | Prop[] | no | Props specific to this sub-component |
| `must_contain` | string[] | no | Children that must be present inside this sub-component |
| `allowed_children` | string[] | no | Valid direct children |
| `allowed_parents` | string[] | no | Valid parent components — the validator uses this for composition checks |

## Examples

```json
{
  "title": "Basic usage",
  "description": "Default button with variants",
  "code": "<Button>Click me</Button>\n<Button variant=\"destructive\">Delete</Button>"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `title` | string | yes | Example title |
| `description` | string | no | What the example demonstrates |
| `code` | string | yes | TSX code snippet |

## Tokens

Design tokens represent the design system's primitive values. The MCP `get_tokens` tool exposes these to AI agents.

```json
{
  "name": "background",
  "value": "hsl(var(--background))",
  "category": "color"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Token name |
| `value` | string | yes | Token value (CSS value, hex, HSL, etc.) |
| `category` | string | yes | Grouping (`color`, `spacing`, `radius`, `font`, etc.) |

## Guidelines

Guidelines are composition rules and accessibility requirements. They can be scoped globally (top-level `guidelines[]`) or per-component.

```json
{
  "rule": "alert-dialog-must-have-title",
  "description": "AlertDialogContent must contain an AlertDialogTitle for accessibility",
  "severity": "error",
  "component": "AlertDialog"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `rule` | string | yes | Unique rule identifier (kebab-case) |
| `description` | string | yes | Human-readable explanation |
| `severity` | string | yes | `error`, `warning`, or `info` |
| `component` | string | no | Scope to a specific component (omit for global rules) |

## Validation rules

When UISpec loads a catalog, it validates:

- `name` and `version` are non-empty
- No duplicate component, sub-component, or category names
- Every component has `import_path` and at least one `imported_names` entry
- Component `category` references a defined category
- Sub-component `allowed_parents` reference defined components or sub-components
- Guideline `severity` is one of `error`, `warning`, `info`

Run `uispec inspect <Component> --catalog your-catalog.json` to verify it loads correctly.

## Automating catalog generation

If your component library uses TypeScript, [react-docgen-typescript](https://github.com/styleguidist/react-docgen-typescript) can extract prop types automatically. You'll need a script to transform its output into the UISpec schema:

```bash
# 1. Extract raw prop data
npx react-docgen-typescript src/components/ui/*.tsx --out raw-docs.json

# 2. Transform to UISpec format (write your own mapper script)
node scripts/to-uispec-catalog.js raw-docs.json > catalog.json

# 3. Test it
uispec inspect Button --catalog catalog.json
uispec validate src/pages/index.tsx --catalog catalog.json
```

The mapper script needs to:
1. Map each component to a `Component` object with `name`, `import_path`, `imported_names`
2. Map each prop's `tsType` to `type`, and enum values to `allowed_values`
3. Add `categories`, `sub_components`, `examples`, and `guidelines` manually (react-docgen-typescript doesn't produce these)

## Bundled catalogs

UISpec ships with a bundled shadcn/ui catalog embedded in the binary:

| Catalog | Components | Categories | Path |
|---|---|---|---|
| shadcn/ui | 30 | 7 | `catalogs/shadcn/catalog.json` |

The bundled catalog is used automatically when no `--catalog` flag is given and no `.uispec/config.yaml` exists.

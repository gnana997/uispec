# UISpec

**Give AI agents deep knowledge of your component library.**

[![Go Report Card](https://goreportcard.com/badge/github.com/gnana997/uispec)](https://goreportcard.com/report/github.com/gnana997/uispec)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.24-blue)](go.mod)
[![Status: Alpha](https://img.shields.io/badge/status-alpha-orange)](https://github.com/gnana997/uispec)

UISpec is a Go binary that runs as an [MCP server](https://modelcontextprotocol.io) for AI coding agents. It gives agents structured access to a UI component catalog — what components exist, what props they accept, what values are valid — and validates agent-generated code against that catalog using tree-sitter parsing.

Works as a CLI tool for developers and as an MCP server for AI agents. One binary, no runtime dependencies.

---

## How it works

UISpec operates in two modes:

**As an MCP server** — the agent queries the catalog through purpose-built tools during planning and code generation, then calls `validate_page` to check its output before writing to disk. Errors are caught in milliseconds without burning extra generation tokens.

**As a CLI tool** — developers run `uispec validate` in CI or during development to catch component misuse, wrong prop values, and incorrect imports.

---

## MCP Tools

Nine tools covering the full agent workflow:

| Tool | Purpose |
|---|---|
| `list_categories` | Browse the component library structure |
| `list_components` | Filter components by category or keyword |
| `get_component_details` | Full prop schemas, import paths, sub-components (batched) |
| `get_component_examples` | Code examples for a specific component |
| `get_tokens` | Design tokens filtered by category |
| `get_guidelines` | Composition rules and accessibility requirements |
| `search_components` | Full-text search across names, descriptions, and props |
| `validate_page` | Parse TSX code and validate all component usages against the catalog |
| `analyze_page` | Compact structural summary of a page for modification planning |

`validate_page` supports `auto_fix: true` — deterministic errors (wrong import paths, invalid enum values) are corrected and the fixed code is returned directly.

---

## Quickstart

**Install:**

```bash
# Homebrew (macOS / Linux)
brew install gnana997/tap/uispec

# Or download a pre-built binary from GitHub Releases
# https://github.com/gnana997/uispec/releases

# Or build from source
go install github.com/gnana997/uispec/cmd/uispec@latest
```

**Initialize a project** (writes `.uispec/config.yaml` and extracts the bundled shadcn catalog):

```bash
uispec init
```

**Validate a file against the catalog:**

```bash
uispec validate src/pages/dashboard.tsx
uispec validate src/pages/dashboard.tsx --fix   # auto-fix deterministic errors
```

**Look up a component:**

```bash
uispec inspect Button
uispec inspect Dialog
uispec inspect DialogContent   # sub-component lookup
```

**Start the MCP server:**

```bash
uispec serve
```

---

## Add to your AI agent

### Claude Desktop

```json
{
  "mcpServers": {
    "uispec": {
      "command": "uispec",
      "args": ["serve"]
    }
  }
}
```

### Claude Code

```bash
claude mcp add uispec -- uispec serve
```

### Cursor

Add to `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project):

```json
{
  "mcpServers": {
    "uispec": {
      "command": "uispec",
      "args": ["serve"]
    }
  }
}
```

### OpenAI Codex

```bash
codex mcp add uispec -- uispec serve
```

<details>
<summary>VS Code (GitHub Copilot)</summary>

Add to `.vscode/mcp.json`:

```json
{
  "servers": {
    "uispec": {
      "type": "stdio",
      "command": "uispec",
      "args": ["serve"]
    }
  }
}
```

</details>

UISpec looks for `.uispec/config.yaml` in the current directory. Run `uispec init` once in your project root and the server picks up your catalog automatically.

---

## CLI Reference

### `uispec init`

Sets up UISpec in the current project. Writes `.uispec/config.yaml` and extracts the bundled shadcn/ui catalog to `.uispec/catalogs/shadcn.json` by default.

```bash
uispec init                          # shadcn preset (default)
uispec init --catalog my-catalog.json  # custom catalog path
uispec init --force                  # overwrite existing config
```

### `uispec validate`

Parses a TSX file and validates every component usage against the catalog. Exits `0` for clean, `2` for violations.

```bash
uispec validate src/pages/landing.tsx
uispec validate src/pages/landing.tsx --fix    # apply deterministic fixes in-place
uispec validate src/pages/landing.tsx --json   # machine-readable output
uispec validate src/pages/landing.tsx --catalog path/to/catalog.json
```

**Violation types detected:**

- Unknown component (not in catalog)
- Missing import / wrong import path
- Invalid prop value (not in allowed enum)
- Unknown prop (not defined for component)
- Missing required prop
- Composition violation (e.g. `CardContent` outside `Card`)
- Deprecated component or prop

### `uispec inspect`

Look up a component's props, allowed values, sub-components, and guidelines.

```bash
uispec inspect Button
uispec inspect Dialog --examples
uispec inspect CardContent --json
uispec inspect Button --catalog path/to/catalog.json
```

### `uispec serve`

Start the MCP server on stdio (used by Claude Desktop, Cursor, VS Code, and any MCP-compatible client).

```bash
uispec serve                                # uses bundled shadcn catalog (zero-config)
uispec serve --log                          # log MCP calls to .uispec/logs/mcp.jsonl
uispec serve --log-file /tmp/uispec.log     # log to a custom path
uispec serve --catalog path/to/custom.json  # use a custom catalog
```

**Logging:** When `--log` or `--log-file` is enabled, every MCP tool call is recorded as a JSONL entry with tool name, sanitized params, duration, response size, and estimated tokens. Useful for debugging and submitting with bug reports. Large params like `code` are replaced with byte lengths for privacy.

```jsonl
{"ts":"2026-02-26T12:00:00Z","tool":"validate_page","params":{"auto_fix":false,"code_len":1200},"duration_ms":12,"response_bytes":843,"tokens_est":211,"error":null}
```

---

## Catalog

UISpec ships with a bundled [shadcn/ui](https://ui.shadcn.com) catalog embedded in the binary. Running `uispec init` extracts it to your project.

**Current bundled catalog:**

- 30 components across 7 categories (actions, feedback, forms, layout, navigation, overlay, data-display)
- Full prop schemas with types, required flags, defaults, and allowed values
- Sub-component composition rules (e.g. `DialogContent` must contain `DialogTitle`)
- Import paths, design tokens, accessibility guidelines

You can also point UISpec at any hand-curated `catalog.json` using `--catalog`. See the [Catalog Format Reference](catalogs/README.md) for the full schema, field descriptions, and automation tips.

---

## Roadmap

| Item | Status |
|---|---|
| shadcn/ui catalog (30 components) | ✅ Shipped |
| MCP server with 9 tools | ✅ Shipped |
| TSX validation engine (10 rules + auto-fix) | ✅ Shipped |
| CLI: `init`, `validate`, `inspect`, `serve` | ✅ Shipped |
| TSX component scanner (`uispec scan`) | 🔜 Next |
| Full shadcn/ui catalog (all components) | 🔜 Next |
| Radix UI catalog | 🔜 Planned |

---

## Catalog format

UISpec uses an open JSON schema. You can hand-author a catalog for any component library and point UISpec at it.

<details>
<summary>Minimal catalog.json example</summary>

```json
{
  "components": [
    {
      "name": "Button",
      "description": "A clickable button element.",
      "category": "actions",
      "import_path": "@/components/ui/button",
      "imported_names": ["Button"],
      "props": [
        {
          "name": "variant",
          "type": "string",
          "required": false,
          "default": "default",
          "allowed_values": ["default", "destructive", "outline", "ghost"]
        }
      ]
    }
  ],
  "categories": [
    { "name": "actions", "components": ["Button"] }
  ],
  "tokens": [],
  "guidelines": []
}
```

</details>

---

## Contributing

Issues and PRs are welcome. To run tests:

```bash
go test ./...
go vet ./...
```

---

## License

MIT — see [LICENSE](LICENSE).

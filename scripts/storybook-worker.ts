/**
 * Storybook CSF parser worker for uispec.
 *
 * Reads a JSON payload from stdin: { files: string[] }
 * Writes JSON to stdout: { results: StoryFileResult[] }
 *
 * Uses @storybook/csf-tools to parse .stories.tsx files and extract
 * component name, story names, and generate JSX examples from args.
 */

import { loadCsf, extractSource, type CsfFile } from "@storybook/csf-tools";
import { readFileSync } from "fs";

/**
 * Convert a PascalCase export name to a human-readable story name.
 * e.g., "MyStoryName" → "My Story Name"
 */
function storyNameFromExport(exportName: string): string {
  return exportName
    .replace(/([A-Z])/g, " $1")
    .replace(/^ /, "")
    .trim();
}

// ── Types ──────────────────────────────────────────────────────────────

interface Input {
  files: string[];
}

interface StoryFileResult {
  filePath: string;
  componentName: string;
  componentImport: string;
  title: string;
  description: string;
  stories: StoryInfo[];
}

interface StoryInfo {
  name: string;
  exportName: string;
  code: string;
  hasPlayFunction: boolean;
  hasRenderFunction: boolean;
}

// ── Helpers ────────────────────────────────────────────────────────────

/**
 * Resolve the import path for a given component identifier from the AST.
 */
function resolveComponentImport(
  csf: CsfFile,
  componentName: string
): string {
  if (!componentName) return "";
  const ast = csf._ast;
  for (const node of ast.program.body) {
    if (node.type !== "ImportDeclaration") continue;
    for (const spec of node.specifiers) {
      if (
        (spec.type === "ImportSpecifier" ||
          spec.type === "ImportDefaultSpecifier") &&
        spec.local.name === componentName
      ) {
        return node.source.value;
      }
    }
  }
  return "";
}

/**
 * Extract simple literal value from a Babel AST node.
 * Returns [value, ok] — ok is false for complex expressions.
 */
function extractLiteral(
  node: any
): [string | number | boolean | null, boolean] {
  if (!node) return [null, false];
  switch (node.type) {
    case "StringLiteral":
      return [node.value, true];
    case "NumericLiteral":
      return [node.value, true];
    case "BooleanLiteral":
      return [node.value, true];
    case "NullLiteral":
      return [null, true];
    case "TemplateLiteral":
      // Only handle simple template literals with no expressions
      if (node.expressions.length === 0 && node.quasis.length === 1) {
        return [node.quasis[0].value.cooked, true];
      }
      return [null, false];
    default:
      return [null, false];
  }
}

/**
 * Extract args key-value pairs from a story's AST node.
 * Only collects simple literal values (string, number, boolean).
 */
function extractArgs(
  csf: CsfFile,
  storyKey: string
): Map<string, { value: any; type: string }> {
  const args = new Map<string, { value: any; type: string }>();
  const storyExport = csf.getStoryExport(storyKey);
  if (!storyExport) return args;

  // Find the 'args' property in the story object
  let storyObj: any = null;
  if (storyExport.type === "VariableDeclarator" && storyExport.init) {
    storyObj = storyExport.init;
  } else if (storyExport.type === "ObjectExpression") {
    storyObj = storyExport;
  }

  // Handle `{ ... } satisfies Story` pattern
  if (storyObj?.type === "TSSatisfiesExpression") {
    storyObj = storyObj.expression;
  }
  // Handle `{ ... } as Story` pattern
  if (storyObj?.type === "TSAsExpression") {
    storyObj = storyObj.expression;
  }

  if (storyObj?.type !== "ObjectExpression") return args;

  for (const prop of storyObj.properties) {
    if (
      prop.type !== "ObjectProperty" ||
      prop.key.type !== "Identifier" ||
      prop.key.name !== "args"
    ) {
      continue;
    }
    if (prop.value.type !== "ObjectExpression") break;

    for (const argProp of prop.value.properties) {
      // Skip spread elements
      if (argProp.type !== "ObjectProperty") continue;
      const keyName =
        argProp.key.type === "Identifier"
          ? argProp.key.name
          : argProp.key.type === "StringLiteral"
            ? argProp.key.value
            : null;
      if (!keyName) continue;

      const [val, ok] = extractLiteral(argProp.value);
      if (ok && val !== null) {
        args.set(keyName, { value: val, type: typeof val });
      }
    }
    break;
  }

  return args;
}

/**
 * Extract args from the meta (default export) object.
 * These serve as default args inherited by all stories.
 */
function extractMetaArgs(
  csf: CsfFile
): Map<string, { value: any; type: string }> {
  const args = new Map<string, { value: any; type: string }>();
  const ast = csf._ast;

  for (const node of ast.program.body) {
    if (node.type !== "ExportDefaultDeclaration") continue;

    let metaObj: any = node.declaration;
    // Handle `{ ... } satisfies Meta` pattern
    if (metaObj?.type === "TSSatisfiesExpression") metaObj = metaObj.expression;
    // Handle `{ ... } as Meta` pattern
    if (metaObj?.type === "TSAsExpression") metaObj = metaObj.expression;

    if (metaObj?.type !== "ObjectExpression") break;

    for (const prop of metaObj.properties) {
      if (
        prop.type !== "ObjectProperty" ||
        prop.key.type !== "Identifier" ||
        prop.key.name !== "args"
      ) {
        continue;
      }
      if (prop.value.type !== "ObjectExpression") break;

      for (const argProp of prop.value.properties) {
        if (argProp.type !== "ObjectProperty") continue;
        const keyName =
          argProp.key.type === "Identifier"
            ? argProp.key.name
            : argProp.key.type === "StringLiteral"
              ? argProp.key.value
              : null;
        if (!keyName) continue;

        const [val, ok] = extractLiteral(argProp.value);
        if (ok && val !== null) {
          args.set(keyName, { value: val, type: typeof val });
        }
      }
      break;
    }
    break;
  }

  return args;
}

/**
 * Check if a story has a render function in its AST.
 */
function hasRenderFn(csf: CsfFile, storyKey: string): boolean {
  const storyExport = csf.getStoryExport(storyKey);
  if (!storyExport) return false;

  let storyObj: any =
    storyExport.type === "VariableDeclarator" ? storyExport.init : storyExport;
  if (storyObj?.type === "TSSatisfiesExpression") storyObj = storyObj.expression;
  if (storyObj?.type === "TSAsExpression") storyObj = storyObj.expression;
  if (storyObj?.type !== "ObjectExpression") return false;

  return storyObj.properties.some(
    (p: any) =>
      p.type === "ObjectProperty" &&
      p.key.type === "Identifier" &&
      p.key.name === "render"
  );
}

/**
 * Extract the render function body as source code.
 */
function extractRenderSource(csf: CsfFile, storyKey: string): string {
  const storyExport = csf.getStoryExport(storyKey);
  if (!storyExport) return "";

  let storyObj: any =
    storyExport.type === "VariableDeclarator" ? storyExport.init : storyExport;
  if (storyObj?.type === "TSSatisfiesExpression") storyObj = storyObj.expression;
  if (storyObj?.type === "TSAsExpression") storyObj = storyObj.expression;
  if (storyObj?.type !== "ObjectExpression") return "";

  for (const prop of storyObj.properties) {
    if (
      prop.type === "ObjectProperty" &&
      prop.key.type === "Identifier" &&
      prop.key.name === "render"
    ) {
      try {
        return extractSource(prop.value).trim();
      } catch {
        return "";
      }
    }
  }
  return "";
}

/**
 * Generate JSX code from component name and args.
 * e.g., <Button variant="primary" size="lg">Click me</Button>
 */
function generateJSX(
  componentName: string,
  args: Map<string, { value: any; type: string }>
): string {
  const children = args.get("children");
  const propsEntries: string[] = [];

  for (const [key, { value, type }] of args) {
    if (key === "children") continue;
    if (type === "string") {
      propsEntries.push(`${key}="${value}"`);
    } else if (type === "number") {
      propsEntries.push(`${key}={${value}}`);
    } else if (type === "boolean" && value === true) {
      propsEntries.push(key);
    } else if (type === "boolean" && value === false) {
      propsEntries.push(`${key}={false}`);
    }
  }

  const propsStr = propsEntries.length > 0 ? " " + propsEntries.join(" ") : "";

  if (children && children.type === "string") {
    return `<${componentName}${propsStr}>${children.value}</${componentName}>`;
  }
  return `<${componentName}${propsStr} />`;
}

/**
 * Extract a nested property value from an ObjectExpression by dot-path.
 * e.g., getNestedStringProp(obj, ["parameters", "docs", "description", "component"])
 */
function getNestedStringProp(obj: any, path: string[]): string {
  let current = obj;
  for (const key of path) {
    if (current?.type !== "ObjectExpression") return "";
    const prop = current.properties.find(
      (p: any) =>
        p.type === "ObjectProperty" &&
        ((p.key.type === "Identifier" && p.key.name === key) ||
          (p.key.type === "StringLiteral" && p.key.value === key))
    );
    if (!prop) return "";
    current = prop.value;
  }
  const [val, ok] = extractLiteral(current);
  if (ok && typeof val === "string") return val;
  return "";
}

/**
 * Extract component description from the CSF meta object.
 * Checks (in order):
 *   1. meta.parameters.docs.description.component
 *   2. meta.description (non-standard but sometimes used)
 */
function extractMetaDescription(csf: CsfFile): string {
  const ast = csf._ast;
  for (const node of ast.program.body) {
    if (node.type !== "ExportDefaultDeclaration") continue;

    let metaObj: any = node.declaration;
    if (metaObj?.type === "TSSatisfiesExpression") metaObj = metaObj.expression;
    if (metaObj?.type === "TSAsExpression") metaObj = metaObj.expression;
    if (metaObj?.type !== "ObjectExpression") break;

    // Try parameters.docs.description.component first.
    const docsDesc = getNestedStringProp(metaObj, [
      "parameters",
      "docs",
      "description",
      "component",
    ]);
    if (docsDesc) return docsDesc;

    // Fallback: top-level description field.
    const topDesc = getNestedStringProp(metaObj, ["description"]);
    if (topDesc) return topDesc;

    break;
  }
  return "";
}

// ── Main ───────────────────────────────────────────────────────────────

function processFile(filePath: string): StoryFileResult | null {
  let code: string;
  try {
    code = readFileSync(filePath, "utf-8");
  } catch (err) {
    process.stderr.write(`warn: cannot read ${filePath}: ${err}\n`);
    return null;
  }

  let csf: CsfFile & { meta: any; stories: any[] };
  try {
    csf = loadCsf(code, {
      fileName: filePath,
      makeTitle: (t: string) => t,
    }).parse() as any;
  } catch (err) {
    process.stderr.write(`warn: cannot parse ${filePath}: ${err}\n`);
    return null;
  }

  const componentName = csf.meta?.component || "";
  if (!componentName) {
    // No component reference — skip
    return null;
  }

  const componentImport = resolveComponentImport(csf, componentName);
  const title = csf.meta?.title || "";
  const description = extractMetaDescription(csf);

  // _stories maps export names (e.g., "Primary") to internal data.
  // csf.stories[] has id/name/__stats but NOT the export name.
  // We iterate _stories keys to get export names, then cross-reference
  // with csf.stories[] for display name and stats.
  const _storiesKeys = Object.keys((csf as any)._stories || {});
  const storiesByName = new Map(
    csf.stories.map((s: any) => [s.name, s])
  );

  // Extract meta-level args (inherited by all stories).
  const metaArgs = extractMetaArgs(csf);

  const stories: StoryInfo[] = [];
  for (const exportName of _storiesKeys) {
    const displayName = storyNameFromExport(exportName);
    // Find matching entry in csf.stories by display name
    const storyEntry = storiesByName.get(displayName) as any;
    const hasPlay = storyEntry?.__stats?.play === true;
    const hasRender = storyEntry?.__stats?.render === true;

    let code: string;
    if (hasRender) {
      // Extract render function source
      code = extractRenderSource(csf, exportName);
    } else {
      // Merge meta args with story-specific args (story wins on conflict).
      const storyArgs = extractArgs(csf, exportName);
      const merged = new Map(metaArgs);
      for (const [k, v] of storyArgs) {
        merged.set(k, v);
      }
      code = generateJSX(componentName, merged);
    }

    if (!code) {
      // Fallback: simple usage with no args
      code = `<${componentName} />`;
    }

    stories.push({
      name: displayName,
      exportName,
      code,
      hasPlayFunction: hasPlay,
      hasRenderFunction: hasRender,
    });
  }

  if (stories.length === 0) return null;

  return {
    filePath,
    componentName,
    componentImport,
    title,
    description,
    stories,
  };
}

// ── stdin/stdout I/O ───────────────────────────────────────────────────

const chunks: Buffer[] = [];
process.stdin.on("data", (chunk) => chunks.push(chunk));
process.stdin.on("end", () => {
  try {
    const input: Input = JSON.parse(Buffer.concat(chunks).toString("utf-8"));
    const results: StoryFileResult[] = [];

    for (const filePath of input.files) {
      const result = processFile(filePath);
      if (result) {
        results.push(result);
      }
    }

    process.stdout.write(JSON.stringify({ results }));
  } catch (err) {
    process.stderr.write(`fatal: ${err}\n`);
    process.exit(1);
  }
});

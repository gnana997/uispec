/**
 * docgen-worker.ts — Node.js worker for react-docgen-typescript prop extraction.
 *
 * Reads JSON from stdin: { files: string[], tsconfig: string }
 * Writes JSON to stdout: DocgenResult[]
 *
 * Bundled via esbuild and embedded in the Go binary.
 */

import * as docgen from "react-docgen-typescript";

interface Input {
  files: string[];
  tsconfig: string;
}

interface DocgenProp {
  name: string;
  type: string;
  required: boolean;
  defaultValue: string;
  description: string;
  deprecated: boolean;
  allowedValues: string[] | null;
  parent: string;
}

interface DocgenResult {
  displayName: string;
  filePath: string;
  description: string;
  props: DocgenProp[];
}

// HTML/DOM/SVG intrinsic parent types from @types/react.
// Props from these types are noise (onClick, aria-label, tabIndex, etc.).
// We block these while allowing all library-specific props (Radix, Headless UI, etc.).
const HTML_INTRINSIC_PARENTS = new Set([
  "Attributes",
  "RefAttributes",
  "ClassAttributes",
  "DOMAttributes",
  "AriaAttributes",
  "HTMLAttributes",
  "SVGAttributes",
  "SVGProps",
  "HTMLProps",
  "AllHTMLAttributes",
  "SVGLineElementAttributes",
  "SVGTextElementAttributes",
]);

function isHtmlIntrinsicParent(name: string): boolean {
  return HTML_INTRINSIC_PARENTS.has(name) || name.endsWith("HTMLAttributes");
}

function run(): void {
  let inputData = "";
  process.stdin.setEncoding("utf8");

  process.stdin.on("data", (chunk: string) => {
    inputData += chunk;
  });

  process.stdin.on("end", () => {
    try {
      const input: Input = JSON.parse(inputData);

      if (!input.files || input.files.length === 0) {
        process.stdout.write("[]");
        return;
      }

      // Create parser with tsconfig for full type resolution.
      const parser = docgen.withCustomConfig(input.tsconfig, {
        shouldExtractLiteralValuesFromEnum: true,
        shouldExtractValuesFromUnion: true,
        shouldRemoveUndefinedFromOptional: true,
        savePropValueAsString: true,
        shouldIncludePropTagMap: true,
        propFilter: (prop: docgen.PropItem): boolean => {
          // Filter out props from HTML/DOM/SVG intrinsic types (onClick, aria-*, etc.)
          // but allow props from component libraries (Radix, Headless UI, etc.).
          if (prop.parent) {
            return !isHtmlIntrinsicParent(prop.parent.name);
          }
          return true;
        },
      });

      // Parse ALL files in one call — creates one ts.Program.
      const docs = parser.parse(input.files);

      // Convert to our output format.
      const results: DocgenResult[] = docs.map((doc) => ({
        displayName: doc.displayName,
        filePath: doc.filePath,
        description: doc.description || "",
        props: Object.values(doc.props).map((prop) => {
          // Extract allowed values from enum/union types.
          let allowedValues: string[] | null = null;
          if (prop.type.value && Array.isArray(prop.type.value)) {
            const raw = prop.type.value.map(
              (v: { value: string }) => {
                // Strip surrounding quotes from string literal values.
                const val = v.value;
                if (
                  (val.startsWith('"') && val.endsWith('"')) ||
                  (val.startsWith("'") && val.endsWith("'"))
                ) {
                  return val.slice(1, -1);
                }
                return val;
              }
            );

            // Filter out spurious allowed_values:
            // 1. Boolean expansion ("false", "true") — noise for boolean props.
            const isBooleanExpansion =
              raw.length === 2 &&
              raw.includes("false") &&
              raw.includes("true");
            // 2. Single non-quoted value — means TS expanded a type name, not a real union.
            //    e.g. `string` becomes { name: "enum", value: [{ value: "string" }] }
            const isSingleTypeExpansion =
              raw.length === 1 && !prop.type.value[0].value.startsWith('"') && !prop.type.value[0].value.startsWith("'");

            if (!isBooleanExpansion && !isSingleTypeExpansion) {
              // Only keep values that look like string literals (user-defined unions).
              // Filter out complex type expansions (ReactNode constituents, CSSProperties, etc.)
              const filtered = raw.filter((v: string) => {
                // Keep if the original was a quoted string literal.
                const original = prop.type.value.find(
                  (tv: { value: string }) => {
                    const stripped = tv.value.replace(/^['"]|['"]$/g, "");
                    return stripped === v;
                  }
                );
                if (!original) return false;
                const ov = original.value;
                return (
                  (ov.startsWith('"') && ov.endsWith('"')) ||
                  (ov.startsWith("'") && ov.endsWith("'"))
                );
              });
              allowedValues = filtered.length > 0 ? filtered : null;
            }
          }

          // Check for @deprecated tag.
          const deprecated =
            prop.tags !== undefined && "deprecated" in prop.tags;

          // Default value handling.
          let defaultValue = "";
          if (prop.defaultValue !== null && prop.defaultValue !== undefined) {
            const dv =
              typeof prop.defaultValue === "object"
                ? prop.defaultValue.value
                : prop.defaultValue;
            if (dv !== undefined && dv !== null) {
              defaultValue = String(dv);
              // Strip surrounding quotes.
              if (
                (defaultValue.startsWith('"') && defaultValue.endsWith('"')) ||
                (defaultValue.startsWith("'") && defaultValue.endsWith("'"))
              ) {
                defaultValue = defaultValue.slice(1, -1);
              }
            }
          }

          return {
            name: prop.name,
            type: prop.type.name,
            required: prop.required,
            defaultValue,
            description: prop.description || "",
            deprecated,
            allowedValues,
            parent: prop.parent?.name || "",
          };
        }),
      }));

      process.stdout.write(JSON.stringify(results));
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      process.stderr.write(`docgen-worker error: ${msg}\n`);
      process.exit(1);
    }
  });
}

run();
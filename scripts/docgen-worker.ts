/**
 * docgen-worker.ts — Node.js worker for react-docgen-typescript prop extraction.
 *
 * Reads JSON from stdin: { files: string[], tsconfig: string }
 * Writes JSON to stdout: DocgenResult[]
 *
 * Bundled via esbuild and embedded in the Go binary.
 */

import * as docgen from "react-docgen-typescript";
import * as ts from "typescript";
import * as path from "path";

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

// Essential HTML props that should pass through even when their parent is an
// HTML intrinsic type. Kept minimal to avoid flooding every component with noise.
// Universal props like id, className, style, role, tabIndex are omitted because
// they apply to ALL components and are filtered out by the catalog builder anyway.
const ESSENTIAL_HTML_PROPS = new Set([
  // Form elements — the props developers actually look up in a catalog
  "disabled", "required", "readOnly", "placeholder", "name",
  "checked", "defaultChecked", "type", "autoFocus",
  "autoComplete", "maxLength", "minLength", "max", "min", "step",
  "pattern", "multiple", "rows", "cols",
  // Links / media
  "href", "target", "src", "alt",
  // Labels
  "htmlFor",
]);

// ─── Type Resolution via TS Compiler API ─────────────────────────────────────

// Types that are already fully resolved — don't try to expand these.
const KNOWN_RESOLVED_TYPES = new Set([
  // Primitives
  "string", "number", "boolean", "any", "void", "never", "undefined",
  "null", "object", "unknown", "symbol", "bigint",
  // React types we treat as opaque
  "ReactNode", "ReactElement", "JSX.Element", "CSSProperties",
  "Ref", "RefObject", "MutableRefObject",
  "HTMLElement", "Element", "Node", "EventTarget",
  // Our simplified types
  "function", "array", "enum",
]);

/**
 * Checks if a type name from docgen looks unresolved and should be
 * resolved via the TS Compiler API.
 */
function needsTypeResolution(typeName: string): boolean {
  // Indexed access: SomeType['prop']
  if (typeName.includes("['")) return true;
  // Qualified indexed access: React.ComponentPropsWithoutRef<typeof X>['prop']
  if (typeName.includes("['")) return true;
  // Single PascalCase identifier not in known types → likely a type alias
  if (/^[A-Z][a-zA-Z0-9]*$/.test(typeName) && !KNOWN_RESOLVED_TYPES.has(typeName)) {
    return true;
  }
  return false;
}

interface ResolvedType {
  name: string;
  value?: Array<{ value: string }>;
}

/**
 * Finds a type symbol by name in the scope of a source file.
 * Searches through file-level symbols and imported symbols.
 * Falls back to searching all program source files if not found locally.
 */
function findTypeSymbol(
  checker: ts.TypeChecker,
  sourceFile: ts.SourceFile,
  typeName: string,
  program?: ts.Program
): ts.Symbol | undefined {
  // Use getSymbolsInScope at the source file level — includes imports.
  const symbols = checker.getSymbolsInScope(
    sourceFile,
    ts.SymbolFlags.Type | ts.SymbolFlags.Interface | ts.SymbolFlags.TypeAlias
  );
  const found = symbols.find((s) => s.getName() === typeName);
  if (found) return found;

  // Fallback: search all source files in the program.
  // This handles cross-package types (e.g., RovingFocusGroupProps from another package).
  if (program) {
    for (const sf of program.getSourceFiles()) {
      if (sf === sourceFile) continue;
      // Skip node_modules declaration files to avoid false matches and slowness.
      if (sf.fileName.includes("node_modules")) continue;
      const sfSymbols = checker.getSymbolsInScope(
        sf,
        ts.SymbolFlags.Type | ts.SymbolFlags.Interface | ts.SymbolFlags.TypeAlias
      );
      const match = sfSymbols.find((s) => s.getName() === typeName);
      if (match) return match;
    }
  }

  return undefined;
}

/**
 * Converts a resolved ts.Type into our format (name + optional value array).
 */
function typeToDocgenFormat(resolvedType: ts.Type, checker: ts.TypeChecker): ResolvedType {
  // Check for function type (has call signatures).
  const callSigs = resolvedType.getCallSignatures();
  if (callSigs.length > 0) {
    return { name: "function" };
  }

  // Check for union type.
  if (resolvedType.isUnion()) {
    // Filter out `undefined` from union members — optional props already capture
    // this via `required: false`, so `T | undefined` should resolve to just `T`.
    const members = resolvedType.types.filter(
      (m) => (m.flags & ts.TypeFlags.Undefined) === 0
    );
    if (members.length === 0) return { name: "undefined" };
    if (members.length === 1) {
      // Simplified to a single type after stripping undefined.
      return typeToDocgenFormat(members[0], checker);
    }

    // Check if all members are string literals.
    const allStringLiterals = members.every(
      (m) => m.isStringLiteral()
    );
    if (allStringLiterals) {
      return {
        name: "enum",
        value: members.map((m) => ({
          value: `"${(m as ts.StringLiteralType).value}"`,
        })),
      };
    }

    // Check if it's boolean (true | false).
    const isBool = members.length === 2 && members.every(
      (m) => (m.flags & ts.TypeFlags.BooleanLiteral) !== 0
    );
    if (isBool) {
      return { name: "boolean" };
    }

    // Check for boolean type flag directly.
    if (members.some((m) => (m.flags & ts.TypeFlags.Boolean) !== 0)) {
      // Mixed union with boolean — return the full type string.
      return {
        name: "enum",
        value: members.map((m) => ({
          value: checker.typeToString(m),
        })),
      };
    }

    // Check if all members are number literals.
    const allNumberLiterals = members.every(
      (m) => m.isNumberLiteral()
    );
    if (allNumberLiterals) {
      return {
        name: "enum",
        value: members.map((m) => ({
          value: `"${(m as ts.NumberLiteralType).value}"`,
        })),
      };
    }

    // Mixed union — return as string representation.
    const typeStr = checker.typeToString(
      resolvedType,
      undefined,
      ts.TypeFormatFlags.NoTruncation
    );
    return { name: typeStr };
  }

  // Check for number/string/boolean flags.
  if (resolvedType.flags & ts.TypeFlags.Number) return { name: "number" };
  if (resolvedType.flags & ts.TypeFlags.String) return { name: "string" };
  if (resolvedType.flags & ts.TypeFlags.Boolean) return { name: "boolean" };
  if (resolvedType.flags & ts.TypeFlags.BigInt) return { name: "number" };

  // Fallback: use typeToString.
  const typeStr = checker.typeToString(
    resolvedType,
    undefined,
    ts.TypeFormatFlags.NoTruncation
  );
  return { name: typeStr };
}

/**
 * Resolves an indexed access type like SomeType['prop'] using the TS checker.
 */
function resolveIndexedAccess(
  typeName: string,
  checker: ts.TypeChecker,
  sourceFile: ts.SourceFile,
  program?: ts.Program
): ResolvedType | null {
  // Parse: BaseType['key'] — may have nested generics in base.
  const bracketIdx = typeName.indexOf("['");
  if (bracketIdx === -1) return null;

  const baseName = typeName.slice(0, bracketIdx);
  const keyMatch = typeName.match(/\['([\w-]+)'\]$/);
  if (!keyMatch) return null;
  const propKey = keyMatch[1];

  // For qualified names like React.AriaAttributes, try the full qualified lookup.
  // For complex bases like React.ComponentPropsWithoutRef<...>, skip.
  if (baseName.includes("<") || baseName.includes("(")) return null;

  // Handle qualified names (e.g., React.AriaAttributes → AriaAttributes).
  const simpleName = baseName.includes(".") ? baseName.split(".").pop()! : baseName;
  const symbol = findTypeSymbol(checker, sourceFile, simpleName, program);
  if (!symbol) return null;

  const baseType = checker.getDeclaredTypeOfSymbol(symbol);
  const propSymbol = baseType.getProperty(propKey);
  if (!propSymbol) return null;

  // getTypeOfSymbol gives us the actual type of the property.
  const propType = checker.getTypeOfSymbol(propSymbol);
  return typeToDocgenFormat(propType, checker);
}

/**
 * Resolves a type alias like Orientation, CheckedState, Direction.
 */
function resolveTypeAlias(
  typeName: string,
  checker: ts.TypeChecker,
  sourceFile: ts.SourceFile,
  program?: ts.Program
): ResolvedType | null {
  const symbol = findTypeSymbol(checker, sourceFile, typeName, program);
  if (!symbol) return null;

  const declaredType = checker.getDeclaredTypeOfSymbol(symbol);
  return typeToDocgenFormat(declaredType, checker);
}

/**
 * Resolves an unresolved type using the TS Compiler API.
 * Returns the resolved type info, or null if resolution fails.
 */
function resolveType(
  typeName: string,
  filePath: string,
  checker: ts.TypeChecker,
  program: ts.Program
): ResolvedType | null {
  const sourceFile = program.getSourceFile(filePath);
  if (!sourceFile) return null;

  if (typeName.includes("['")) {
    return resolveIndexedAccess(typeName, checker, sourceFile, program);
  }

  return resolveTypeAlias(typeName, checker, sourceFile, program);
}

/**
 * Second-pass type resolution: scans all docgen results for unresolved types
 * and resolves them via the TS Compiler API. Mutates PropItem in place.
 */
function resolveUnresolvedTypes(
  docs: docgen.ComponentDoc[],
  program: ts.Program
): number {
  const checker = program.getTypeChecker();
  let resolved = 0;

  for (const doc of docs) {
    for (const prop of Object.values(doc.props)) {
      const typeName = prop.type.name;
      if (!needsTypeResolution(typeName)) continue;

      const result = resolveType(typeName, doc.filePath, checker, program);
      if (!result) continue;

      // Overwrite the docgen PropItemType in place.
      prop.type.name = result.name;
      if (result.value) {
        prop.type.value = result.value;
      }
      resolved++;
    }
  }

  return resolved;
}

// ─── Main ────────────────────────────────────────────────────────────────────

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

      // Parse tsconfig ourselves so we can share the ts.Program between
      // react-docgen-typescript and our type resolution pass.
      const configFile = ts.readConfigFile(input.tsconfig, ts.sys.readFile);
      if (configFile.error) {
        throw new Error(`Failed to read tsconfig: ${ts.flattenDiagnosticMessageText(configFile.error.messageText, "\n")}`);
      }
      const basePath = path.dirname(input.tsconfig);
      const parsed = ts.parseJsonConfigFileContent(
        configFile.config,
        ts.sys,
        basePath,
        {},
        input.tsconfig
      );

      // Create a shared program. Include all input files so docgen can find them.
      const allFiles = [...new Set([...parsed.fileNames, ...input.files])];
      let program: ts.Program | undefined;
      const programProvider = (): ts.Program => {
        if (!program) {
          program = ts.createProgram(allFiles, parsed.options);
        }
        return program;
      };

      // Create parser with compiler options (not withCustomConfig, since we
      // parsed the tsconfig ourselves to share the program).
      const parser = docgen.withCompilerOptions(parsed.options, {
        shouldExtractLiteralValuesFromEnum: true,
        shouldExtractValuesFromUnion: true,
        shouldRemoveUndefinedFromOptional: true,
        savePropValueAsString: true,
        shouldIncludePropTagMap: true,
        propFilter: (prop: docgen.PropItem): boolean => {
          if (prop.parent) {
            if (isHtmlIntrinsicParent(prop.parent.name)) {
              return ESSENTIAL_HTML_PROPS.has(prop.name);
            }
          }
          return true;
        },
      });

      // Parse ALL files in one call, sharing our program.
      const docs = parser.parseWithProgramProvider(input.files, programProvider);

      // Second pass: resolve unresolved types via TS Compiler API.
      if (program) {
        const resolvedCount = resolveUnresolvedTypes(docs, program);
        if (resolvedCount > 0) {
          process.stderr.write(`resolved ${resolvedCount} types via TS Compiler API\n`);
        }
      }

      // Convert to our output format.
      const results: DocgenResult[] = docs.map((doc) => ({
        displayName: doc.displayName,
        filePath: doc.filePath,
        description: doc.description || "",
        props: Object.values(doc.props).map((prop) => {
          // Extract allowed values from enum/union types.
          let allowedValues: string[] | null = null;
          // Resolved type name — may be overridden from "enum" to a more specific type.
          let resolvedType = prop.type.name;

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

            if (isBooleanExpansion) {
              // enum with [false, true] → boolean type.
              resolvedType = "boolean";
            } else if (isSingleTypeExpansion) {
              // Single non-quoted value: detect function signatures and known types.
              const val = raw[0];
              if (val.includes("=>")) {
                resolvedType = "function";
              } else if (val === "number" || val === "boolean" || val === "string") {
                resolvedType = val;
              }
              // Otherwise leave as-is (will become "string" via simplifyDocgenType for "enum").
            } else {
              // Check if all values are function signatures (overloaded callbacks).
              const allFunctions = raw.every((v: string) => v.includes("=>"));
              if (allFunctions) {
                resolvedType = "function";
              } else {
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
          } else if (resolvedType === "enum") {
            // Enum with no value array — type couldn't be expanded.
            // Infer from prop name: callbacks starting with "on" are functions.
            if (/^on[A-Z]/.test(prop.name)) {
              resolvedType = "function";
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
            type: resolvedType,
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
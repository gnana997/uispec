/**
 * tokens-worker.ts — Node.js worker for CSS design token extraction via PostCSS.
 *
 * Reads JSON from stdin: { cssFiles: string[] }
 * Writes JSON to stdout: { tokens: TokenResult[], darkMode: boolean }
 *
 * Bundled via esbuild and embedded in the Go binary.
 */

import * as postcss from "postcss";
import * as fs from "fs";

interface Input {
  cssFiles: string[];
}

interface TokenResult {
  name: string;
  value: string;
  category: string;
}

interface TokenOutput {
  tokens: TokenResult[];
  darkMode: boolean;
}

// Variables to skip — framework internals, not design tokens.
const NOISE_PREFIXES = ["--tw-", "--_", "--webkit-", "--moz-"];

function isNoise(prop: string): boolean {
  return NOISE_PREFIXES.some((prefix) => prop.startsWith(prefix));
}

// @theme namespace prefix → category mapping (Tailwind v4).
const THEME_PREFIX_CATEGORIES: Record<string, string> = {
  "--color-": "color",
  "--spacing-": "spacing",
  "--font-": "typography",
  "--text-": "typography",
  "--leading-": "typography",
  "--tracking-": "typography",
  "--radius-": "border",
  "--shadow-": "shadow",
  "--animate-": "animation",
  "--ease-": "animation",
  "--breakpoint-": "breakpoint",
  "--container-": "layout",
  "--inset-": "spacing",
};

// Name pattern → category mapping (for :root variables without namespace).
const NAME_PATTERNS: Array<{ pattern: RegExp; category: string }> = [
  // Color tokens (most common).
  {
    pattern:
      /^--(background|foreground|primary|secondary|accent|destructive|muted|popover|card|input|ring)($|-)/,
    category: "color",
  },
  // Chart tokens.
  { pattern: /^--chart-/, category: "chart" },
  // Sidebar tokens.
  { pattern: /^--sidebar-/, category: "sidebar" },
  // Border/radius tokens.
  { pattern: /^--(radius|border)($|-)/, category: "border" },
  // Spacing tokens.
  { pattern: /^--(spacing|gap|padding|margin)-/, category: "spacing" },
  // Typography tokens.
  {
    pattern: /^--(font|text|line-height|letter-spacing|leading|tracking)-/,
    category: "typography",
  },
  // Shadow tokens.
  { pattern: /^--shadow($|-)/, category: "shadow" },
];

// Value-based heuristics (fallback).
function categoryFromValue(value: string): string | null {
  const v = value.trim().toLowerCase();
  if (
    v.startsWith("oklch(") ||
    v.startsWith("hsl(") ||
    v.startsWith("hsla(") ||
    v.startsWith("rgb(") ||
    v.startsWith("rgba(") ||
    /^#[0-9a-f]{3,8}$/i.test(v)
  ) {
    return "color";
  }
  return null;
}

function categorizeToken(
  prop: string,
  _value: string,
  themeHint: string | null
): string {
  // Priority 1: @theme namespace hint.
  if (themeHint) {
    return themeHint;
  }

  // Priority 2: Name pattern matching.
  for (const { pattern, category } of NAME_PATTERNS) {
    if (pattern.test(prop)) {
      return category;
    }
  }

  // Priority 3: Value heuristic.
  const fromValue = categoryFromValue(_value);
  if (fromValue) {
    return fromValue;
  }

  return "other";
}

/**
 * Extract the referenced variable name from a var() expression.
 * e.g., "var(--background)" → "--background"
 */
function extractVarRef(value: string): string | null {
  const match = value.match(/^var\(\s*(--[\w-]+)\s*\)$/);
  return match ? match[1] : null;
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

      if (!input.cssFiles || input.cssFiles.length === 0) {
        const output: TokenOutput = { tokens: [], darkMode: false };
        process.stdout.write(JSON.stringify(output));
        return;
      }

      // Collected data across all files.
      const rootTokens = new Map<string, string>(); // --name → value
      const themeHints = new Map<string, string>(); // --name → category
      let darkMode = false;

      for (const cssFile of input.cssFiles) {
        let cssContent: string;
        try {
          cssContent = fs.readFileSync(cssFile, "utf-8");
        } catch {
          // Skip unreadable files.
          continue;
        }

        let root: postcss.Root;
        try {
          root = postcss.parse(cssContent, { from: cssFile });
        } catch {
          // Skip unparseable files.
          continue;
        }

        // Walk rules for :root declarations.
        root.walkRules((rule) => {
          const selector = rule.selector.trim();

          // Detect dark mode.
          if (selector === ".dark" || selector.includes(".dark")) {
            darkMode = true;
            return; // Don't extract dark values.
          }

          // Only extract from :root.
          if (selector !== ":root") {
            return;
          }

          rule.walkDecls(/^--/, (decl) => {
            if (isNoise(decl.prop)) return;
            // First occurrence wins (across multiple files).
            if (!rootTokens.has(decl.prop)) {
              rootTokens.set(decl.prop, decl.value);
            }
          });
        });

        // Walk @theme at-rules for categorization hints.
        root.walkAtRules("theme", (atRule) => {
          atRule.walkDecls(/^--/, (decl) => {
            if (isNoise(decl.prop)) return;

            // Determine category from @theme namespace prefix.
            let category: string | null = null;
            for (const [prefix, cat] of Object.entries(
              THEME_PREFIX_CATEGORIES
            )) {
              if (decl.prop.startsWith(prefix)) {
                category = cat;
                break;
              }
            }

            if (category) {
              // Find which :root variable this maps to.
              const varRef = extractVarRef(decl.value);
              if (varRef) {
                // e.g., @theme { --color-background: var(--background) }
                // → hint: --background is a "color"
                themeHints.set(varRef, category);
              } else {
                // Direct value in @theme (e.g., --color-primary: hsl(...))
                // Store as both a token and its own hint.
                if (!rootTokens.has(decl.prop)) {
                  rootTokens.set(decl.prop, decl.value);
                }
                themeHints.set(decl.prop, category);
              }
            }
          });
        });

        // Detect dark mode in @media rules.
        root.walkAtRules("media", (atRule) => {
          if (atRule.params.includes("prefers-color-scheme: dark")) {
            darkMode = true;
          }
        });
      }

      // Build final token list.
      const tokens: TokenResult[] = [];
      for (const [prop, value] of rootTokens) {
        const hint = themeHints.get(prop) || null;
        const category = categorizeToken(prop, value, hint);
        // Strip the -- prefix for the token name.
        const name = prop.replace(/^--/, "");

        tokens.push({ name, value, category });
      }

      // Sort tokens by category, then by name.
      tokens.sort((a, b) => {
        if (a.category !== b.category) return a.category.localeCompare(b.category);
        return a.name.localeCompare(b.name);
      });

      const output: TokenOutput = { tokens, darkMode };
      process.stdout.write(JSON.stringify(output));
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      process.stderr.write(`tokens-worker error: ${msg}\n`);
      process.exit(1);
    }
  });
}

run();

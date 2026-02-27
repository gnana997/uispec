/**
 * build.mjs — Bundles docgen-worker.ts into a single .js file via esbuild.
 *
 * Usage: node scripts/build.mjs
 *
 * The output is a self-contained JS file that only needs `node` to run.
 * TypeScript compiler is bundled in (~5-7MB output).
 */

import * as esbuild from "esbuild";
import { fileURLToPath } from "url";
import { dirname, join } from "path";

const __dirname = dirname(fileURLToPath(import.meta.url));

await esbuild.build({
  entryPoints: [join(__dirname, "docgen-worker.ts")],
  bundle: true,
  platform: "node",
  target: "node18",
  format: "cjs",
  outfile: join(__dirname, "dist", "docgen-worker.js"),
  // Bundle everything including typescript compiler — zero runtime deps.
  external: [],
  minify: true,
  // Suppress warnings about dynamic requires in typescript compiler.
  logLevel: "warning",
});

console.log("Built scripts/dist/docgen-worker.js");

await esbuild.build({
  entryPoints: [join(__dirname, "tokens-worker.ts")],
  bundle: true,
  platform: "node",
  target: "node18",
  format: "cjs",
  outfile: join(__dirname, "dist", "tokens-worker.js"),
  external: [],
  minify: true,
  logLevel: "warning",
});

console.log("Built scripts/dist/tokens-worker.js");
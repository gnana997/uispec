/**
 * build.mjs — Bundles worker scripts into self-contained JS files via esbuild.
 *
 * Usage: node scripts/build.mjs
 *
 * The output files only need `node` to run — zero runtime deps.
 */

import * as esbuild from "esbuild";
import { readFileSync, writeFileSync } from "fs";
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
  external: [],
  minify: true,
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

// Storybook worker: Babel/Storybook internals reference import.meta.url and have
// dynamic require("semver") calls via Module.createRequire that fail when the
// bundle runs from /tmp (no node_modules nearby). We build as CJS with
// import.meta.url polyfilled, then post-process to intercept require("semver").
// NOTE: We intentionally do NOT minify so that the post-processing regex can
// reliably find the bundled semver wrapper variable name.
await esbuild.build({
  entryPoints: [join(__dirname, "storybook-worker.ts")],
  bundle: true,
  platform: "node",
  target: "node18",
  format: "cjs",
  outfile: join(__dirname, "dist", "storybook-worker.cjs"),
  external: [],
  minify: false,
  logLevel: "warning",
  banner: {
    js: "var import_meta_url = typeof document === 'undefined' ? require('url').pathToFileURL(__filename).href : undefined;",
  },
  define: {
    "import.meta.url": "import_meta_url",
  },
});

// Post-process: Monkey-patch Module.createRequire so all require functions
// created by Babel/recast intercept require("semver") and return the bundled
// version instead of going through Node's module resolution.
const sbPath = join(__dirname, "dist", "storybook-worker.cjs");
let sbCode = readFileSync(sbPath, "utf-8");

// Find the __commonJS-wrapped semver/index.js variable name in the output.
// Non-minified pattern: var require_semver2 = __commonJS({\n  "node_modules/semver/index.js"
// We match the full semver package (index.js), not just semver/classes/semver.js.
const semverMatch = sbCode.match(/var (\w+)\s*=\s*\w+\(\{\s*"node_modules\/semver\/index\.js"/);
if (semverMatch) {
  const semverFn = semverMatch[1];
  // Insert after the banner line: override Module.createRequire to wrap all
  // dynamically created require functions with a semver interceptor.
  const bannerEnd = sbCode.indexOf("\n");
  const patch = `;(function(){var M=require("module"),_cr=M.createRequire;M.createRequire=function(u){var r=_cr.call(this,u);return function(m){if(m==="semver")return ${semverFn}();return r.apply(this,arguments)}}})();`;
  sbCode = sbCode.slice(0, bannerEnd + 1) + patch + sbCode.slice(bannerEnd + 1);
  writeFileSync(sbPath, sbCode);
  console.log(`Patched storybook-worker.cjs: require("semver") → ${semverFn}()`);
} else {
  console.warn("Warning: could not find bundled semver in storybook-worker.cjs");
}

console.log("Built scripts/dist/storybook-worker.cjs");

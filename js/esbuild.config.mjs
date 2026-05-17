// Bundles src/index.mjs into a single CommonJS file that Javy can compile to WASM.
// Javy / QuickJS does not provide a DOM, browser globals, or React rendering.
// @platejs/markdown only needs the headless editor + remark pipeline at runtime,
// so we shim what little of React it touches at import time.

import { build } from "esbuild";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { readFileSync } from "node:fs";

const here = dirname(fileURLToPath(import.meta.url));
const banner = readFileSync(resolve(here, "src/banner.js"), "utf8");

await build({
  entryPoints: [resolve(here, "src/index.mjs")],
  bundle: true,
  format: "iife",
  platform: "neutral",
  target: "es2020",
  outfile: resolve(here, "dist/bundle.js"),
  mainFields: ["browser", "module", "main"],
  conditions: ["browser", "import", "default"],
  external: [],
  alias: {
    // Plate's React-side imports compile in but are never executed during
    // serialize / deserialize. Point them at lightweight stubs so the bundle
    // does not pull in the React renderer / scheduler.
    "react": resolve(here, "src/shim/react.mjs"),
    "react-dom": resolve(here, "src/shim/react-dom.mjs"),
    "react-dom/client": resolve(here, "src/shim/react-dom.mjs"),
    "react-compiler-runtime": resolve(here, "src/shim/react-compiler-runtime.mjs"),
  },
  define: {
    "process.env.NODE_ENV": JSON.stringify("production"),
  },
  // We never render: math nodes only need their plugin metadata, not the
  // katex stylesheet / fonts. Drop these on the floor at bundle time.
  loader: {
    ".css": "empty",
    ".woff": "empty",
    ".woff2": "empty",
    ".ttf": "empty",
    ".eot": "empty",
    ".svg": "empty",
    ".png": "empty",
  },
  banner: { js: banner },
  legalComments: "none",
  logLevel: "info",
});

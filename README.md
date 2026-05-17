# go-platemd-wasm

Convert between [Plate](https://platejs.org/) JSON values and Markdown from Go,
using the upstream [`@platejs/markdown`](https://www.npmjs.com/package/@platejs/markdown)
library — compiled to WebAssembly with [Javy](https://github.com/bytecodealliance/javy),
embedded in the binary, and executed in-process by the pure-Go
[wazero](https://wazero.io/) runtime.

No Node.js or CGO at runtime. A single static Go binary ships everything.

```go
import platemd "github.com/shelojara/go-platemd-wasm"

c, err := platemd.New()
if err != nil { /* ... */ }
defer c.Close()

value, err := c.MarkdownToPlate(ctx, "# Hello\n\nWorld **here**.\n", nil)
// value is []map[string]any matching Plate's element schema:
// [{type:"h1", children:[{text:"Hello"}]}, {type:"p", children:[...]}]

md, err := c.PlateToMarkdown(ctx, value, nil)
// md == "# Hello\n\nWorld **here**.\n"
```

## Install

```sh
go get github.com/shelojara/go-platemd-wasm
```

The `.wasm` blob is checked into the repo, so `go get` is all you need.

## API

```go
// Compile the embedded WASM module once and reuse the Converter across calls.
func New() (*Converter, error)
func (*Converter) Close() error

// Single conversions. nil Options uses the default plugin set.
func (*Converter) MarkdownToPlate(ctx, md string, *Options) ([]Node, error)
func (*Converter) PlateToMarkdown(ctx, value []Node, *Options) (string, error)

// Batch packs many ops into one WASM invocation, amortizing the ~400ms
// JS-runtime startup across the whole batch.
func (*Converter) Batch(ctx, []BatchOp) ([]BatchResult, error)

// Throwaway one-shot helpers — convenient, but they compile a fresh WASM
// module every time. Prefer New() + reuse for hot paths.
func MarkdownToPlate(ctx, md string) ([]Node, error)
func PlateToMarkdown(ctx, value []Node) (string, error)
```

### Options

```go
type Options struct {
    // Disable removes whole plugin categories from the editor that processes
    // this call. Valid categories: "basic", "marks", "lists", "links",
    // "code", "tables", "media".
    Disable []string

    // Markdown is forwarded into MarkdownPlugin.configure({ options: ... })
    // on the JS side. See @platejs/markdown for accepted keys
    // (`nodes`, `rules`, `remarkPlugins`, etc.).
    Markdown map[string]any
}
```

## Performance

Rough numbers on `linux/amd64`:

| Phase                         | Cost      |
|-------------------------------|-----------|
| `New()` — wazero compile      | ~600 ms   |
| Single call — JS engine boot  | ~400–700 ms |
| Single call — actual work     | a few ms  |
| Batch of N — fixed cost       | one engine boot for the whole batch |

If you're converting thousands of documents, batch them. If you do one
conversion per request, the JS boot dominates.

## How it works

```
+----------------+      +------------------+      +----------------+
|    your Go     |--->  |   wazero runs    |--->  | bundled JS:    |
|  platemd.New() |      |   plate.wasm     |      | @platejs/      |
|                |<---  |  (Javy/QuickJS)  |<---  |   markdown     |
+----------------+      +------------------+      +----------------+
        stdin / stdout = newline-free JSON request / response
```

1. `js/src/index.mjs` instantiates a headless Plate editor with the
   markdown plugin plus a curated set of element plugins (headings,
   lists, links, code blocks, tables, media, basic marks). It reads one
   JSON request from stdin, runs the conversion, and writes one JSON
   response to stdout.
2. `esbuild` bundles that into a single file, with shims for `react`,
   `react-dom`, browser globals (`document`, `crypto.getRandomValues`,
   etc.) that Plate touches at import time but never invokes during
   serialize / deserialize.
3. `javy` compiles the bundle into a WASI module containing QuickJS.
4. `internal/wasm/plate.wasm` is embedded via `//go:embed`. The Go side
   uses wazero to instantiate it once per call, piping JSON in and out.

## Rebuilding the WASM blob

The blob is committed so consumers don't need a JS toolchain. To regenerate it:

```sh
# Install javy 8.x: https://github.com/bytecodealliance/javy/releases
make js-install   # one-time
make wasm
```

That runs `esbuild` then `javy build` and overwrites `internal/wasm/plate.wasm`.

## Limitations

- **No math plugin (KaTeX).** `@platejs/math` pulls in KaTeX, which has
  module-load code that assumes a real DOM (`document.compatMode` etc.)
  and crashes under Javy. Math is not in the default plugin set;
  re-enabling it needs a more thorough KaTeX stub.
- **No custom Plate plugins from Go.** You can disable categories and
  pass `MarkdownPlugin` options, but adding a wholly new plugin requires
  editing `js/src/index.mjs` and rebuilding.
- **Startup-bound.** Each call boots a QuickJS instance with the full
  Plate bundle (~400 ms). Use `Batch()` for throughput.
- **Lossy edges.** `remark-stringify` normalizes output (italic `*…*`
  becomes `_…_`, unordered list `-` becomes `*`, etc.). Content
  round-trips; exact byte equality does not.

## License

The Go and JS source under this repo is MIT. The embedded WASM blob
contains compiled copies of `@platejs/markdown`, `platejs`, `remark`,
`mdast-util-*`, and QuickJS (via Javy) — each under their own license.
See `js/package.json` for the dependency list.

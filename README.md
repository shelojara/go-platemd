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

The `.wasm` blob (~3 MB) is checked into the repo, so `go get` is all you need.

### Default options

If every conversion in your process should share the same plugin /
markdown settings, set them once at construction time. Per-call options
still override field-by-field:

```go
c, _ := platemd.New(platemd.WithDefaultOptions(platemd.Options{
    Disable:  []string{"tables", "media"},   // strip these out by default
    Markdown: map[string]any{
        // anything accepted by MarkdownPlugin.configure({ options: ... })
    },
}))
defer c.Close()

// Uses the defaults — fast path on a Worker (cached editor).
value, _ := c.MarkdownToPlate(ctx, md, nil)

// Per-call override: this call re-enables tables, defaults untouched
// for other calls.
value2, _ := c.MarkdownToPlate(ctx, md, &platemd.Options{Disable: nil})
```

A Worker created from a Converter inherits the defaults via a one-shot
`set_defaults` op at startup — so default-options calls hit the cached
editor, no per-call rebuild.

## API

```go
// Compile the embedded WASM module once and reuse the Converter across calls.
func New() (*Converter, error)
func (*Converter) Close() error

// Single conversions. Each call boots a fresh JS engine (~400ms). For
// repeated calls, use a Worker (see below).
func (*Converter) MarkdownToPlate(ctx, md string, *Options) ([]Node, error)
func (*Converter) PlateToMarkdown(ctx, value []Node, *Options) (string, error)

// Batch packs many ops into one WASM invocation, amortizing the JS
// engine boot across the whole batch.
func (*Converter) Batch(ctx, []BatchOp) ([]BatchResult, error)

// Worker — keep one WASM instance alive for low-latency repeated calls.
// Pays the ~400ms boot once at NewWorker(); each subsequent call is
// single-digit milliseconds.
func (*Converter) NewWorker(ctx) (*Worker, error)
func (*Worker) MarkdownToPlate(ctx, md string, *Options) ([]Node, error)
func (*Worker) PlateToMarkdown(ctx, value []Node, *Options) (string, error)
func (*Worker) Batch(ctx, []BatchOp) ([]BatchResult, error)
func (*Worker) Close() error

// Throwaway one-shot helpers — convenient, but they compile a fresh WASM
// module every time. Prefer New() + Worker for hot paths.
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
    // (`allowedNodes`, `disallowedNodes`, `rules`, `remarkStringifyOptions`,
    // etc.). `remarkPlugins` cannot be set from Go — remark plugins are
    // functions and don't survive the JSON boundary. The bundled set
    // (currently just `remark-gfm`) is fixed at build time in
    // `js/src/index.mjs`.
    Markdown map[string]any
}
```

`remark-gfm` is enabled by default, so GFM tables, strikethrough
(`~~foo~~`), task lists (`- [x] done`), and bare-URL autolinks all parse
and serialize. Tables are nested as `table` → `tr` → `th`/`td` → `p`
(see [examples/05-table.json](./examples/05-table.json)).

## Examples

A few worked pairs live in [`examples/`](./examples) so you can see what
the Plate JSON looks like for common markdown shapes. Each file pair is
the result of one `MarkdownToPlate` call on the default plugin set:

| Markdown | JSON | What it covers |
| --- | --- | --- |
| [01-formatting.md](./examples/01-formatting.md) | [01-formatting.json](./examples/01-formatting.json) | heading + paragraph with `bold` / `italic` / `code` marks |
| [02-lists.md](./examples/02-lists.md) | [02-lists.json](./examples/02-lists.json) | unordered + ordered lists (indent-based `listStyleType` shape) |
| [03-link-quote-code.md](./examples/03-link-quote-code.md) | [03-link-quote-code.json](./examples/03-link-quote-code.json) | inline link, blockquote, fenced code block |
| [04-image-and-rule.md](./examples/04-image-and-rule.md) | [04-image-and-rule.json](./examples/04-image-and-rule.json) | image (`img` with `caption`) and horizontal rule |
| [05-table.md](./examples/05-table.md) | [05-table.json](./examples/05-table.json) | GFM table (`table` / `tr` / `th` / `td` nesting) |

For instance, `01-formatting.md`:

```md
# Welcome

This paragraph has **bold**, *italic*, and `inline code` text.
```

becomes:

```json
[
  { "type": "h1", "children": [{ "text": "Welcome" }] },
  {
    "type": "p",
    "children": [
      { "text": "This paragraph has " },
      { "text": "bold", "bold": true },
      { "text": ", " },
      { "text": "italic", "italic": true },
      { "text": ", and " },
      { "text": "inline code", "code": true },
      { "text": " text." }
    ]
  }
]
```

Regenerate the fixtures after changing the plugin set with:

```sh
go run ./cmd/gen-examples
```

## Performance

Two things make `Worker` dramatically faster than the one-shot `Converter`
path:

1. The QuickJS runtime is loaded once at `NewWorker()` and reused for
   every subsequent call — no per-call WASM instantiation.
2. The default Plate editor (with all plugins) is constructed once at
   JS module load and reused across calls; only calls that pass
   `*Options` build a fresh editor.

Workers serialize their calls with an internal mutex (one in-flight
request at a time). For parallel throughput, create one `*Worker` per
goroutine.

```go
c, _ := platemd.New()
defer c.Close()

w, _ := c.NewWorker(ctx)   // one-time ~950ms boot
defer w.Close()

for _, doc := range docs {
    value, _ := w.MarkdownToPlate(ctx, doc, nil)  // ~14ms each
    // ...
}
```

### Benchmark results

Measured on `linux/amd64` (Intel Xeon @ 2.1 GHz), `go test -bench`:

```
BenchmarkConverter_MdToPlate_Small        641 ms/op
BenchmarkConverter_PlateToMd_Small        809 ms/op
BenchmarkConverter_Batch10                781 ms/op   (~78 ms per inner op)

BenchmarkWorker_MdToPlate_Small            17 ms/op   << fast path
BenchmarkWorker_MdToPlate_Medium          367 ms/op   (~10× larger doc)
BenchmarkWorker_PlateToMd_Small           192 ms/op
BenchmarkWorker_Batch10                   130 ms/op   (~13 ms per inner op)
BenchmarkWorker_BatchPlateToMd10         1884 ms/op   (~188 ms per inner op)

BenchmarkNew                              578 ms/op   (one-time, per process)
BenchmarkNewWorker                        626 ms/op   (one-time, per worker)
```

Headlines:

- `Worker.MarkdownToPlate` on a small document is **~70× faster** than
  `Converter.MarkdownToPlate` (14 ms vs 965 ms). Most of the one-shot
  cost is just booting the JS engine.
- `plate → md` is consistently the slower direction — `markdown.serialize`
  walks the document with every plugin's rule chain, and that cost is
  per-call rather than once-per-process. Batching does not amortize it
  (it's per-op, not per-boot).
- `Converter.Batch10` shows the one-shot batch story: a single engine
  boot covers 10 ops, so each op effectively costs ~108 ms.

Re-run them yourself:

```sh
go test -run=^$ -bench=. -benchtime=10x -count=1 ./...
```

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
   lists, links, code blocks, tables, media, basic marks). The editor
   is built once at module load. The JS then sits in a `while(true)`
   loop, reading one length-framed JSON request from stdin, running the
   conversion, and writing one length-framed JSON response to stdout.
   The loop exits when stdin reaches EOF — so the same WASM blob serves
   both one-shot mode (Go writes one frame and closes stdin) and worker
   mode (Go keeps the pipe open).
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
- **No MDX.** The build stubs out `remark-mdx` / `mdast-util-mdx` to
  drop ~300 KB of acorn + micromark extensions. Plain markdown is fine;
  documents containing JSX or MDX expressions are not parsed as MDX.
- **No custom Plate plugins from Go.** You can disable categories and
  pass `MarkdownPlugin` options, but adding a wholly new plugin requires
  editing `js/src/index.mjs` and rebuilding.
- **One-shot path is startup-bound.** Each `*Converter` call boots a
  QuickJS instance with the Plate bundle. Use a `*Worker` (or
  `Batch()`) for throughput.
- **Lossy edges.** `remark-stringify` normalizes output (italic `*…*`
  becomes `_…_`, unordered list `-` becomes `*`, etc.). Content
  round-trips; exact byte equality does not.

## License

The Go and JS source under this repo is MIT. The embedded WASM blob
contains compiled copies of `@platejs/markdown`, `platejs`, `remark`,
`mdast-util-*`, and QuickJS (via Javy) — each under their own license.
See `js/package.json` for the dependency list.

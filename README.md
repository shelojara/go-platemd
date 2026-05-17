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
    // (`nodes`, `rules`, `remarkPlugins`, etc.).
    Markdown map[string]any
}
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

Measured on `linux/amd64` (Intel Xeon @ 2.8 GHz), `go test -bench`:

```
BenchmarkConverter_MdToPlate_Small        965 ms/op
BenchmarkConverter_PlateToMd_Small       1159 ms/op
BenchmarkConverter_Batch10               1082 ms/op   (108 ms per inner op)

BenchmarkWorker_MdToPlate_Small            14 ms/op   << fast path
BenchmarkWorker_MdToPlate_Medium          488 ms/op   (~10× larger doc)
BenchmarkWorker_PlateToMd_Small           230 ms/op
BenchmarkWorker_Batch10                   148 ms/op   (15 ms per inner op)
BenchmarkWorker_BatchPlateToMd10         2261 ms/op   (226 ms per inner op)

BenchmarkNew                              707 ms/op   (one-time, per process)
BenchmarkNewWorker                        942 ms/op   (one-time, per worker)
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

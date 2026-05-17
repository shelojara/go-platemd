package platemd

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Converter holds the wazero runtime and compiled WASM module. It is safe
// for concurrent use: each call instantiates its own module instance.
// For low-latency repeated calls, create a *Worker via NewWorker.
type Converter struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	defaults *Options
}

// New compiles the embedded WASM module. The compile step is the most
// expensive part (~600ms on first call); reuse the returned *Converter
// across all conversions for the lifetime of your process.
//
// Pass WithDefaultOptions to set a baseline Options that applies to
// every call (and every Worker spawned from this Converter) unless the
// caller overrides per call.
func New(opts ...Option) (*Converter, error) {
	cfg := converterConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(true))
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("wasi setup: %w", err)
	}
	mod, err := rt.CompileModule(ctx, wasmBlob)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("compile module: %w", err)
	}
	return &Converter{runtime: rt, compiled: mod, defaults: cfg.defaults}, nil
}

// Defaults returns a deep copy of the baseline options the Converter was
// constructed with, or nil if none were set. The returned value is safe
// to mutate; changes do not propagate back into the Converter.
func (c *Converter) Defaults() *Options {
	if c == nil || c.defaults == nil {
		return nil
	}
	out := Options{}
	if c.defaults.Disable != nil {
		out.Disable = append([]string(nil), c.defaults.Disable...)
	}
	if c.defaults.Markdown != nil {
		out.Markdown = make(map[string]any, len(c.defaults.Markdown))
		for k, v := range c.defaults.Markdown {
			out.Markdown[k] = v
		}
	}
	return &out
}

// Close releases the wazero runtime. After Close, the Converter and any
// Workers spawned from it are unusable.
func (c *Converter) Close() error {
	if c == nil || c.runtime == nil {
		return nil
	}
	return c.runtime.Close(context.Background())
}

// opRequest is the JSON envelope sent to the WASM module.
type opRequest struct {
	Op      string   `json:"op"`
	Md      string   `json:"md,omitempty"`
	Plate   []Node   `json:"plate,omitempty"`
	Options *Options `json:"options,omitempty"`
}

type batchEnvelope struct {
	Ops []opRequest `json:"ops"`
}

type wireResult struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
	Stack string          `json:"stack,omitempty"`
}

type batchResponse struct {
	OK      bool         `json:"ok"`
	Error   string       `json:"error,omitempty"`
	Results []wireResult `json:"results,omitempty"`
}

// runOne is the single-shot path: spin up a fresh WASM instance, send
// one frame, read one frame, exit. Used by *Converter methods.
func (c *Converter) runOne(ctx context.Context, req opRequest) (json.RawMessage, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	stdout, err := c.invoke(ctx, payload)
	if err != nil {
		return nil, err
	}
	body, err := decodeFrame(stdout)
	if err != nil {
		return nil, err
	}
	var res wireResult
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !res.OK {
		return nil, fmt.Errorf("plate js: %s", res.Error)
	}
	return res.Data, nil
}

// runBatch sends one batch envelope through a fresh WASM instance.
func (c *Converter) runBatch(ctx context.Context, reqs []opRequest) ([]wireResult, error) {
	payload, err := json.Marshal(batchEnvelope{Ops: reqs})
	if err != nil {
		return nil, fmt.Errorf("marshal batch: %w", err)
	}
	stdout, err := c.invoke(ctx, payload)
	if err != nil {
		return nil, err
	}
	body, err := decodeFrame(stdout)
	if err != nil {
		return nil, err
	}
	var res batchResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}
	if !res.OK {
		return nil, fmt.Errorf("plate js (batch): %s", res.Error)
	}
	return res.Results, nil
}

// invoke instantiates one module instance, writes one length-framed payload,
// reads one length-framed response. Stdin is closed after writing so the
// JS while-loop exits cleanly on the next iteration.
func (c *Converter) invoke(ctx context.Context, payload []byte) ([]byte, error) {
	var stdinBuf bytes.Buffer
	writeFrameTo(&stdinBuf, payload)

	stdin := bytes.NewReader(stdinBuf.Bytes())
	var stdout, stderr bytes.Buffer

	cfg := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs("plate.wasm")

	mod, err := c.runtime.InstantiateModule(ctx, c.compiled, cfg)
	if err != nil {
		return nil, fmt.Errorf("wasm run: %w (stderr: %s)", err, trimStderr(stderr.String()))
	}
	if err := mod.Close(ctx); err != nil {
		return nil, fmt.Errorf("wasm close: %w", err)
	}
	if stdout.Len() == 0 {
		return nil, errors.New("wasm produced no output")
	}
	return stdout.Bytes(), nil
}

// ----- framing helpers (4-byte big-endian length prefix) -----

const frameHeader = 4

func writeFrameTo(w io.Writer, payload []byte) error {
	var hdr [frameHeader]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func readFrameFrom(r io.Reader) ([]byte, error) {
	var hdr [frameHeader]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// decodeFrame extracts one length-prefixed frame from a buffer of stdout
// bytes captured from the one-shot path.
func decodeFrame(b []byte) ([]byte, error) {
	if len(b) < frameHeader {
		return nil, fmt.Errorf("response truncated: %d bytes", len(b))
	}
	n := binary.BigEndian.Uint32(b[:frameHeader])
	if int(n)+frameHeader > len(b) {
		return nil, fmt.Errorf("response truncated: header says %d, have %d", n, len(b)-frameHeader)
	}
	return b[frameHeader : frameHeader+int(n)], nil
}

func trimStderr(s string) string {
	const max = 512
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

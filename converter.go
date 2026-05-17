package platemd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Converter holds the wazero runtime and compiled WASM module. It is safe
// for concurrent use: each call instantiates its own module instance with
// its own stdin / stdout pipes.
type Converter struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

// New compiles the embedded WASM module. The compile step is the most
// expensive part (~600ms on first call); reuse the returned *Converter
// across all conversions for the lifetime of your process.
func New() (*Converter, error) {
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
	return &Converter{runtime: rt, compiled: mod}, nil
}

// Close releases the wazero runtime. After Close, the Converter is unusable.
func (c *Converter) Close() error {
	if c == nil || c.runtime == nil {
		return nil
	}
	return c.runtime.Close(context.Background())
}

// opRequest is the JSON envelope sent to the WASM module on stdin.
// It matches the shape handled by js/src/index.mjs.
type opRequest struct {
	Op      string   `json:"op"`
	Md      string   `json:"md,omitempty"`
	Plate   []Node   `json:"plate,omitempty"`
	Options *Options `json:"options,omitempty"`
}

type batchEnvelope struct {
	Ops []opRequest `json:"ops"`
}

// wireResult is one item out of a WASM response, before per-op decoding.
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

// runOne invokes the WASM module with a single op request and returns the
// `data` payload on success.
func (c *Converter) runOne(ctx context.Context, req opRequest) (json.RawMessage, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	stdout, err := c.invoke(ctx, payload)
	if err != nil {
		return nil, err
	}
	var res wireResult
	if err := json.Unmarshal(stdout, &res); err != nil {
		return nil, fmt.Errorf("decode response (%d bytes): %w", len(stdout), err)
	}
	if !res.OK {
		return nil, fmt.Errorf("plate js: %s", res.Error)
	}
	return res.Data, nil
}

// runBatch invokes the WASM module with a batch envelope and returns the
// raw per-op results.
func (c *Converter) runBatch(ctx context.Context, reqs []opRequest) ([]wireResult, error) {
	payload, err := json.Marshal(batchEnvelope{Ops: reqs})
	if err != nil {
		return nil, fmt.Errorf("marshal batch: %w", err)
	}
	stdout, err := c.invoke(ctx, payload)
	if err != nil {
		return nil, err
	}
	var res batchResponse
	if err := json.Unmarshal(stdout, &res); err != nil {
		return nil, fmt.Errorf("decode batch response (%d bytes): %w", len(stdout), err)
	}
	if !res.OK {
		return nil, fmt.Errorf("plate js (batch): %s", res.Error)
	}
	return res.Results, nil
}

// invoke instantiates one module instance, feeds it the payload on stdin,
// reads stdout, and tears the instance down. Each call is independent;
// invoke is safe to call concurrently from multiple goroutines.
func (c *Converter) invoke(ctx context.Context, payload []byte) ([]byte, error) {
	stdin := bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer

	cfg := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs("plate.wasm")

	mod, err := c.runtime.InstantiateModule(ctx, c.compiled, cfg)
	if err != nil {
		// stderr usually contains the JS stack trace when a guest error
		// surfaces here as wasm trap.
		return nil, fmt.Errorf("wasm run: %w (stderr: %s)", err, trim(stderr.String()))
	}
	if err := mod.Close(ctx); err != nil {
		return nil, fmt.Errorf("wasm close: %w", err)
	}
	if stdout.Len() == 0 {
		return nil, errors.New("wasm produced no output")
	}
	return stdout.Bytes(), nil
}

func trim(s string) string {
	const max = 512
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

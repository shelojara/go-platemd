package platemd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/tetratelabs/wazero"
)

// Worker keeps a single WASM instance alive across many calls, amortizing
// the JS engine startup cost (~400ms cold) over every conversion. A typical
// call through a Worker is single-digit milliseconds.
//
// Concurrency model: each Worker handles one in-flight call at a time;
// methods serialize via an internal mutex. For parallel throughput,
// create one Worker per goroutine. Workers are bound to the *Converter
// that created them — when the Converter is closed, in-flight calls on
// its Workers will fail and Close will return.
type Worker struct {
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader

	// defaults mirrors the Converter's defaults. Per-call options on Worker
	// methods are merged with these field-by-field on the Go side before
	// being sent — but if the caller passes nil, no options field is sent
	// at all so the JS uses its already-configured cached editor.
	defaults *Options

	mu sync.Mutex

	doneOnce sync.Once
	done     chan struct{}
	runErr   error
	closed   bool
}

// NewWorker spawns a long-running WASM instance. Costs roughly one
// instantiation (~400ms) up front; subsequent calls on the Worker
// skip that cost.
func (c *Converter) NewWorker(ctx context.Context) (*Worker, error) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	w := &Worker{
		stdinW:  stdinW,
		stdoutR: stdoutR,
		done:    make(chan struct{}),
	}

	cfg := wazero.NewModuleConfig().
		WithStdin(stdinR).
		WithStdout(stdoutW).
		WithStderr(io.Discard).
		WithArgs("plate.wasm")

	// Wait for the JS loop to be ready before returning. We do this by
	// sending a ping op and reading the response — that proves both the
	// module instantiated and the read/write pipes are wired up.
	ready := make(chan error, 1)

	go func() {
		defer close(w.done)
		defer stdoutW.Close()
		defer stdinR.Close()
		mod, err := c.runtime.InstantiateModule(context.Background(), c.compiled, cfg)
		if mod != nil {
			_ = mod.Close(context.Background())
		}
		w.runErr = err
		select {
		case ready <- err:
		default:
		}
	}()

	// Issue the warm-up ping. If the parent Converter has defaults set,
	// follow with a set_defaults so the cached editor inside JS is
	// rebuilt with those defaults — that preserves the fast path for
	// subsequent calls that don't pass per-call overrides.
	if err := writeFrameTo(stdinW, []byte(`{"op":"ping"}`)); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("worker warm-up write: %w", err)
	}
	body, err := readFrameFrom(stdoutR)
	if err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("worker warm-up read: %w", err)
	}
	var res wireResult
	if err := json.Unmarshal(body, &res); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("worker warm-up parse: %w", err)
	}
	if !res.OK {
		_ = w.Close()
		return nil, fmt.Errorf("worker warm-up: %s", res.Error)
	}

	if c.defaults != nil {
		w.defaults = c.defaults
		payload, _ := json.Marshal(opRequest{Op: "set_defaults", Options: c.defaults})
		if err := writeFrameTo(stdinW, payload); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("worker set_defaults write: %w", err)
		}
		body, err := readFrameFrom(stdoutR)
		if err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("worker set_defaults read: %w", err)
		}
		var r wireResult
		if err := json.Unmarshal(body, &r); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("worker set_defaults parse: %w", err)
		}
		if !r.OK {
			_ = w.Close()
			return nil, fmt.Errorf("worker set_defaults: %s", r.Error)
		}
	}
	// Ignore `ready` — if the goroutine had already errored, the read
	// above would have failed too.
	_ = ready
	return w, nil
}

// effectiveOptions decides what to send as the `options` field. When the
// caller passes nil, we send nil too (no options field) so the JS uses its
// already-configured cached editor — fast path. When the caller passes
// non-nil overrides, we merge with defaults and send the result.
func (w *Worker) effectiveOptions(perCall *Options) *Options {
	if perCall == nil {
		return nil
	}
	return mergeOptions(w.defaults, perCall)
}

// MarkdownToPlate parses markdown into a Plate value using this Worker.
func (w *Worker) MarkdownToPlate(ctx context.Context, md string, opts *Options) ([]Node, error) {
	raw, err := w.call(ctx, opRequest{Op: "md_to_plate", Md: md, Options: w.effectiveOptions(opts)})
	if err != nil {
		return nil, err
	}
	var value []Node
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode plate value: %w", err)
	}
	return value, nil
}

// PlateToMarkdown serializes a Plate value to markdown using this Worker.
func (w *Worker) PlateToMarkdown(ctx context.Context, value []Node, opts *Options) (string, error) {
	raw, err := w.call(ctx, opRequest{Op: "plate_to_md", Plate: value, Options: w.effectiveOptions(opts)})
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("decode markdown string: %w", err)
	}
	return s, nil
}

// Batch packs multiple ops into one round-trip. Compared to Converter.Batch,
// this saves nothing per-op (the worker already amortizes startup) but
// halves the request / response overhead for many ops.
func (w *Worker) Batch(ctx context.Context, ops []BatchOp) ([]BatchResult, error) {
	if len(ops) == 0 {
		return nil, nil
	}
	reqs := make([]opRequest, len(ops))
	for i, op := range ops {
		effective := w.effectiveOptions(op.Options)
		switch {
		case op.Markdown != "" && op.PlateValue != nil:
			return nil, fmt.Errorf("op[%d]: both Markdown and PlateValue set", i)
		case op.PlateValue != nil:
			reqs[i] = opRequest{Op: "plate_to_md", Plate: op.PlateValue, Options: effective}
		default:
			reqs[i] = opRequest{Op: "md_to_plate", Md: op.Markdown, Options: effective}
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	body, err := w.exchange(ctx, batchEnvelope{Ops: reqs})
	if err != nil {
		return nil, err
	}
	var br batchResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}
	if !br.OK {
		return nil, fmt.Errorf("plate js (batch): %s", br.Error)
	}
	if len(br.Results) != len(ops) {
		return nil, fmt.Errorf("batch: got %d results for %d ops", len(br.Results), len(ops))
	}
	out := make([]BatchResult, len(ops))
	for i, r := range br.Results {
		if !r.OK {
			out[i].Err = errors.New(r.Error)
			continue
		}
		if reqs[i].Op == "md_to_plate" {
			if err := json.Unmarshal(r.Data, &out[i].PlateValue); err != nil {
				out[i].Err = fmt.Errorf("decode plate value: %w", err)
			}
		} else {
			if err := json.Unmarshal(r.Data, &out[i].Markdown); err != nil {
				out[i].Err = fmt.Errorf("decode markdown string: %w", err)
			}
		}
	}
	return out, nil
}

// call is the single-op send/receive path.
func (w *Worker) call(ctx context.Context, req opRequest) (json.RawMessage, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	body, err := w.exchange(ctx, req)
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

// exchange writes one framed request and reads one framed response.
// Caller must hold w.mu.
func (w *Worker) exchange(ctx context.Context, v any) ([]byte, error) {
	if w.closed {
		return nil, errors.New("worker closed")
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	// ctx cancellation: shut down the worker so the blocked read/write
	// returns. Otherwise the caller is stuck waiting for the JS to finish.
	if ctx != nil {
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			select {
			case <-ctx.Done():
				_ = w.stdinW.CloseWithError(ctx.Err())
				_ = w.stdoutR.CloseWithError(ctx.Err())
			case <-stop:
			}
		}()
	}

	if err := writeFrameTo(w.stdinW, payload); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	body, err := readFrameFrom(w.stdoutR)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
}

// Close terminates the worker. Pending calls will fail.
func (w *Worker) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	w.doneOnce.Do(func() {
		// Closing stdin signals EOF to the JS read loop, which exits
		// cleanly. The run goroutine then closes stdout and we unblock.
		_ = w.stdinW.Close()
	})
	<-w.done
	return w.runErr
}

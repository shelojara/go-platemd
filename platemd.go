// Package platemd converts between Plate JSON values and Markdown by running
// the upstream @platejs/markdown library compiled to WebAssembly. The WASM
// blob (built with Javy) is embedded in the binary and executed in-process
// by the pure-Go wazero runtime — no Node, no CGO, no external processes.
//
// Cost model: compiling the WASM module takes ~600ms and is done once per
// Converter. Each individual call instantiates a fresh WASM instance
// (~400ms cold) so the JavaScript runtime is loaded per call. When
// converting many documents, use Batch to amortize that startup across
// all of them.
package platemd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Node is one Plate element / text node. A Plate document is []Node.
// Plate's schema is open, so we model it as a plain map.
type Node = map[string]any

// Options configure plugin selection and forwarded MarkdownPlugin config
// for a single conversion. Zero value = default plugin set, no overrides.
type Options struct {
	// Disable removes whole plugin categories from the editor that will
	// process this call. Valid categories: "basic", "marks", "lists",
	// "links", "code", "tables", "media".
	Disable []string `json:"disable,omitempty"`
	// Markdown is forwarded into MarkdownPlugin.configure({ options: ... })
	// on the JS side. See the @platejs/markdown docs for accepted keys
	// (allowedNodes, disallowedNodes, rules, remarkStringifyOptions, ...).
	// remarkPlugins cannot be set from Go — plugin functions don't survive
	// the JSON boundary. The bundled remark plugin set (currently just
	// remark-gfm, so GFM tables / strikethrough / task lists / autolinks
	// all work out of the box) is fixed at build time in js/src/index.mjs.
	Markdown map[string]any `json:"markdown,omitempty"`
}

// BatchOp describes one op inside a Batch call.
type BatchOp struct {
	// Markdown set => md→plate; PlateValue set => plate→md. Exactly one
	// must be populated.
	Markdown   string
	PlateValue []Node
	Options    *Options
}

// BatchResult mirrors a BatchOp by index. Exactly one of PlateValue
// or Markdown is set on success; Err is set on failure.
type BatchResult struct {
	PlateValue []Node
	Markdown   string
	Err        error
}

// MarkdownToPlate is a one-shot helper that builds a temporary Converter,
// runs the conversion, and tears it down. Prefer New()+reuse for repeated
// calls — this helper pays the WASM compile cost every time.
func MarkdownToPlate(ctx context.Context, md string, opts ...Option) ([]Node, error) {
	c, err := New(opts...)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.MarkdownToPlate(ctx, md, nil)
}

// PlateToMarkdown is the one-shot equivalent of (*Converter).PlateToMarkdown.
func PlateToMarkdown(ctx context.Context, value []Node, opts ...Option) (string, error) {
	c, err := New(opts...)
	if err != nil {
		return "", err
	}
	defer c.Close()
	return c.PlateToMarkdown(ctx, value, nil)
}

// --- Converter methods (implementation lives in converter.go) ---

// MarkdownToPlate parses markdown into a Plate value.
func (c *Converter) MarkdownToPlate(ctx context.Context, md string, opts *Options) ([]Node, error) {
	req := opRequest{Op: "md_to_plate", Md: md, Options: mergeOptions(c.defaults, opts)}
	res, err := c.runOne(ctx, req)
	if err != nil {
		return nil, err
	}
	var value []Node
	if err := json.Unmarshal(res, &value); err != nil {
		return nil, fmt.Errorf("decode plate value: %w", err)
	}
	return value, nil
}

// PlateToMarkdown serializes a Plate value to markdown.
func (c *Converter) PlateToMarkdown(ctx context.Context, value []Node, opts *Options) (string, error) {
	req := opRequest{Op: "plate_to_md", Plate: value, Options: mergeOptions(c.defaults, opts)}
	res, err := c.runOne(ctx, req)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(res, &s); err != nil {
		return "", fmt.Errorf("decode markdown string: %w", err)
	}
	return s, nil
}

// Batch packs multiple ops into one WASM invocation. The returned slice
// has the same length and ordering as ops. Per-op failures populate
// Result.Err and do not abort the batch; a returned error from Batch
// itself means the entire invocation failed (e.g. WASM crash).
func (c *Converter) Batch(ctx context.Context, ops []BatchOp) ([]BatchResult, error) {
	if len(ops) == 0 {
		return nil, nil
	}
	reqs := make([]opRequest, len(ops))
	for i, op := range ops {
		effective := mergeOptions(c.defaults, op.Options)
		switch {
		case op.Markdown != "" && op.PlateValue != nil:
			return nil, fmt.Errorf("op[%d]: both Markdown and PlateValue set", i)
		case op.PlateValue != nil:
			reqs[i] = opRequest{Op: "plate_to_md", Plate: op.PlateValue, Options: effective}
		default:
			reqs[i] = opRequest{Op: "md_to_plate", Md: op.Markdown, Options: effective}
		}
	}

	raw, err := c.runBatch(ctx, reqs)
	if err != nil {
		return nil, err
	}
	if len(raw) != len(ops) {
		return nil, fmt.Errorf("batch: got %d results for %d ops", len(raw), len(ops))
	}

	out := make([]BatchResult, len(ops))
	for i, r := range raw {
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

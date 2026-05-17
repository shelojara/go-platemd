// Package pure provides a pure-Go implementation of the Plate JSON
// <-> Markdown conversion that mirrors the WASM-backed parent package.
// It uses goldmark (with the GFM extension set) to parse markdown and
// emits the same Plate node shapes that the @platejs/markdown bundle
// produces — flat indent-based lists, table/tr/th/td/p nesting,
// code_block / code_line, and so on.
//
// Tradeoffs vs the WASM backend:
//
//   - No JS engine boot — every call is single-digit milliseconds. There
//     is no Worker; you don't need one.
//   - No `Options.Markdown` map — that field is forwarded into the JS
//     MarkdownPlugin, which has no analogue here. Use `Disable` to drop
//     plugin categories; everything else is fixed at compile time.
//   - Output is not byte-identical to remark-stringify. Italic uses `_…_`
//     and unordered lists use `* `, matching the WASM backend's
//     normalization, but corner cases (escaping, link titles, mixed
//     marks) may diverge.
package pure

import (
	"context"
	"errors"
	"fmt"
)

// Node is one Plate element / text node. A document is []Node. Mirrors
// the parent package's alias so values pass between the two without
// conversion.
type Node = map[string]any

// Options configure plugin selection for a single conversion. Zero value
// = default plugin set.
type Options struct {
	// Disable removes whole plugin categories. Valid categories:
	// "basic", "marks", "lists", "links", "code", "tables", "media",
	// "mentions". Mirrors the WASM backend's option of the same name.
	// Unknown category names are silently ignored.
	Disable []string `json:"disable,omitempty"`
}

// BatchOp describes one op inside a Batch call. Exactly one of Markdown
// or PlateValue must be set.
type BatchOp struct {
	Markdown   string
	PlateValue []Node
	Options    *Options
}

// BatchResult mirrors a BatchOp by index. Exactly one of PlateValue or
// Markdown is set on success; Err is set on failure.
type BatchResult struct {
	PlateValue []Node
	Markdown   string
	Err        error
}

// Converter is the entry point. It is safe for concurrent use — there
// is no shared mutable state.
type Converter struct {
	defaults *Options
}

// Option configures a *Converter at construction time.
type Option func(*config)

type config struct {
	defaults *Options
}

// WithDefaultOptions sets baseline Options applied to every call unless
// the caller overrides per call. Per-call options replace fields
// non-nil-by-non-nil (same rule as the WASM backend).
func WithDefaultOptions(o Options) Option {
	return func(c *config) {
		cp := o
		c.defaults = &cp
	}
}

// New constructs a Converter. Cheap — no WASM compile, no JS boot.
func New(opts ...Option) (*Converter, error) {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Converter{defaults: cfg.defaults}, nil
}

// Defaults returns a deep copy of the baseline options, or nil if none
// were set.
func (c *Converter) Defaults() *Options {
	if c == nil || c.defaults == nil {
		return nil
	}
	out := Options{}
	if c.defaults.Disable != nil {
		out.Disable = append([]string(nil), c.defaults.Disable...)
	}
	return &out
}

// Close is a no-op; provided for API symmetry with the WASM backend.
func (c *Converter) Close() error { return nil }

// MarkdownToPlate parses markdown into a Plate value.
func (c *Converter) MarkdownToPlate(ctx context.Context, md string, opts *Options) ([]Node, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	effective := mergeOptions(c.defaults, opts)
	return parseMarkdown(md, effective), nil
}

// PlateToMarkdown serializes a Plate value to markdown.
func (c *Converter) PlateToMarkdown(ctx context.Context, value []Node, opts *Options) (string, error) {
	if err := ctxErr(ctx); err != nil {
		return "", err
	}
	effective := mergeOptions(c.defaults, opts)
	return serializeMarkdown(value, effective), nil
}

// Batch runs many ops in a single call. Per-op failures populate
// Result.Err and do not abort the batch.
func (c *Converter) Batch(ctx context.Context, ops []BatchOp) ([]BatchResult, error) {
	if len(ops) == 0 {
		return nil, nil
	}
	out := make([]BatchResult, len(ops))
	for i, op := range ops {
		if op.Markdown != "" && op.PlateValue != nil {
			return nil, fmt.Errorf("op[%d]: both Markdown and PlateValue set", i)
		}
		effective := mergeOptions(c.defaults, op.Options)
		switch {
		case op.PlateValue != nil:
			out[i].Markdown = serializeMarkdown(op.PlateValue, effective)
		default:
			out[i].PlateValue = parseMarkdown(op.Markdown, effective)
		}
	}
	return out, nil
}

// MarkdownToPlate is a one-shot helper.
func MarkdownToPlate(ctx context.Context, md string, opts ...Option) ([]Node, error) {
	c, err := New(opts...)
	if err != nil {
		return nil, err
	}
	return c.MarkdownToPlate(ctx, md, nil)
}

// PlateToMarkdown is the one-shot equivalent of (*Converter).PlateToMarkdown.
func PlateToMarkdown(ctx context.Context, value []Node, opts ...Option) (string, error) {
	c, err := New(opts...)
	if err != nil {
		return "", err
	}
	return c.PlateToMarkdown(ctx, value, nil)
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errors.New("context: " + err.Error())
	}
	return nil
}

func mergeOptions(defaults, perCall *Options) *Options {
	if defaults == nil {
		return perCall
	}
	if perCall == nil {
		out := *defaults
		return &out
	}
	out := *defaults
	if perCall.Disable != nil {
		out.Disable = perCall.Disable
	}
	return &out
}

func disabled(opts *Options, category string) bool {
	if opts == nil || len(opts.Disable) == 0 {
		return false
	}
	for _, c := range opts.Disable {
		if c == category {
			return true
		}
	}
	return false
}

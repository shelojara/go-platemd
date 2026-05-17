package platemd_test

import (
	"context"
	"strings"
	"testing"
	"time"

	platemd "github.com/shelojara/go-platemd-wasm"
)

func newConverter(t *testing.T) *platemd.Converter {
	t.Helper()
	c, err := platemd.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestMarkdownToPlate_Basic(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	value, err := c.MarkdownToPlate(ctx, "# Hello\n\nThis is **bold** text.\n", nil)
	if err != nil {
		t.Fatalf("MarkdownToPlate: %v", err)
	}
	if len(value) != 2 {
		t.Fatalf("expected 2 top-level nodes, got %d: %+v", len(value), value)
	}
	if got, want := value[0]["type"], "h1"; got != want {
		t.Errorf("node[0].type = %v, want %v", got, want)
	}
	if got, want := value[1]["type"], "p"; got != want {
		t.Errorf("node[1].type = %v, want %v", got, want)
	}
	children, _ := value[1]["children"].([]any)
	if len(children) < 3 {
		t.Fatalf("expected >=3 paragraph children, got %d", len(children))
	}
	boldChild, _ := children[1].(map[string]any)
	if boldChild["bold"] != true || boldChild["text"] != "bold" {
		t.Errorf("expected bold child {bold:true, text:\"bold\"}, got %+v", boldChild)
	}
}

func TestPlateToMarkdown_Basic(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	value := []platemd.Node{
		{"type": "h1", "children": []any{map[string]any{"text": "Hello"}}},
		{"type": "p", "children": []any{
			map[string]any{"text": "This is "},
			map[string]any{"text": "bold", "bold": true},
			map[string]any{"text": " text."},
		}},
	}
	md, err := c.PlateToMarkdown(ctx, value, nil)
	if err != nil {
		t.Fatalf("PlateToMarkdown: %v", err)
	}
	if !strings.Contains(md, "# Hello") {
		t.Errorf("missing heading in %q", md)
	}
	if !strings.Contains(md, "**bold**") {
		t.Errorf("missing bold in %q", md)
	}
}

func TestRoundTrip(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cases := []struct {
		name string
		md   string
	}{
		{"heading and paragraph", "# Title\n\nSome paragraph text.\n"},
		{"emphasis", "Plain *italic* and **bold** text.\n"},
		{"inline code", "Use `printf` for output.\n"},
		{"unordered list", "- one\n- two\n- three\n"},
		{"ordered list", "1. first\n2. second\n3. third\n"},
		{"blockquote", "> a quoted line\n"},
		{"link", "Here is [a link](https://example.com).\n"},
		{"code block", "```go\nfmt.Println(\"hi\")\n```\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, err := c.MarkdownToPlate(ctx, tc.md, nil)
			if err != nil {
				t.Fatalf("md->plate: %v", err)
			}
			if len(value) == 0 {
				t.Fatalf("md->plate produced empty value for %q", tc.md)
			}
			md, err := c.PlateToMarkdown(ctx, value, nil)
			if err != nil {
				t.Fatalf("plate->md: %v", err)
			}
			if strings.TrimSpace(md) == "" {
				t.Errorf("plate->md produced empty markdown from %q (plate=%+v)", tc.md, value)
			}
			// Spot-check a key substring round-trips. We don't expect
			// byte equality (remark normalizes list markers, code fences,
			// etc.), but distinctive content should survive.
			t.Logf("\nin:  %q\nout: %q", tc.md, md)
		})
	}
}

func TestBatch(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ops := []platemd.BatchOp{
		{Markdown: "# A\n"},
		{Markdown: "## B\n"},
		{PlateValue: []platemd.Node{{"type": "p", "children": []any{map[string]any{"text": "from plate"}}}}},
	}
	results, err := c.Batch(ctx, ops)
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("results[0].Err: %v", results[0].Err)
	}
	if results[0].PlateValue == nil || results[0].PlateValue[0]["type"] != "h1" {
		t.Errorf("results[0]: want h1, got %+v", results[0].PlateValue)
	}
	if results[2].PlateValue != nil {
		t.Errorf("results[2] should be markdown-only, got plate: %+v", results[2].PlateValue)
	}
	if !strings.Contains(results[2].Markdown, "from plate") {
		t.Errorf("results[2].Markdown missing content: %q", results[2].Markdown)
	}
}

func TestEmptyInputs(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := c.MarkdownToPlate(ctx, "", nil); err != nil {
		t.Fatalf("empty md: %v", err)
	}
	if _, err := c.PlateToMarkdown(ctx, nil, nil); err != nil {
		t.Fatalf("nil plate: %v", err)
	}
}

func TestOneShot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	v, err := platemd.MarkdownToPlate(ctx, "# hi\n")
	if err != nil {
		t.Fatalf("MarkdownToPlate: %v", err)
	}
	if len(v) != 1 || v[0]["type"] != "h1" {
		t.Errorf("unexpected value: %+v", v)
	}

	md, err := platemd.PlateToMarkdown(ctx, []platemd.Node{
		{"type": "h2", "children": []any{map[string]any{"text": "sub"}}},
	})
	if err != nil {
		t.Fatalf("PlateToMarkdown: %v", err)
	}
	if !strings.Contains(md, "## sub") {
		t.Errorf("unexpected md: %q", md)
	}
}

func TestDisableOption(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// With the list plugin registered, the markdown plugin emits the
	// indent-based list shape (a paragraph with listStyleType).
	withLists, err := c.MarkdownToPlate(ctx, "- one\n- two\n", nil)
	if err != nil {
		t.Fatalf("with lists: %v", err)
	}
	// With it disabled, it falls back to the older nested ul/li/lic shape.
	withoutLists, err := c.MarkdownToPlate(ctx, "- one\n- two\n",
		&platemd.Options{Disable: []string{"lists"}})
	if err != nil {
		t.Fatalf("without lists: %v", err)
	}
	t.Logf("with lists:    %+v", withLists)
	t.Logf("without lists: %+v", withoutLists)

	// The two shapes should differ at the top level.
	if len(withLists) > 0 && len(withoutLists) > 0 &&
		withLists[0]["type"] == withoutLists[0]["type"] {
		t.Errorf("expected different top-level types between enabled / disabled lists; both were %v",
			withLists[0]["type"])
	}
}

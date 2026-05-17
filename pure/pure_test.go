package pure_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shelojara/go-platemd-wasm/pure"
)

func newConverter(t *testing.T) *pure.Converter {
	t.Helper()
	c, err := pure.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestMarkdownToPlate_Basic(t *testing.T) {
	c := newConverter(t)
	value, err := c.MarkdownToPlate(context.Background(), "# Hello\n\nThis is **bold** text.\n", nil)
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
	value := []pure.Node{
		{"type": "h1", "children": []any{map[string]any{"text": "Hello"}}},
		{"type": "p", "children": []any{
			map[string]any{"text": "This is "},
			map[string]any{"text": "bold", "bold": true},
			map[string]any{"text": " text."},
		}},
	}
	md, err := c.PlateToMarkdown(context.Background(), value, nil)
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
			value, err := c.MarkdownToPlate(context.Background(), tc.md, nil)
			if err != nil {
				t.Fatalf("md->plate: %v", err)
			}
			if len(value) == 0 {
				t.Fatalf("md->plate produced empty value for %q", tc.md)
			}
			md, err := c.PlateToMarkdown(context.Background(), value, nil)
			if err != nil {
				t.Fatalf("plate->md: %v", err)
			}
			if strings.TrimSpace(md) == "" {
				t.Errorf("plate->md produced empty markdown from %q (plate=%+v)", tc.md, value)
			}
			t.Logf("\nin:  %q\nout: %q", tc.md, md)
		})
	}
}

func TestBatch(t *testing.T) {
	c := newConverter(t)
	ops := []pure.BatchOp{
		{Markdown: "# A\n"},
		{Markdown: "## B\n"},
		{PlateValue: []pure.Node{{"type": "p", "children": []any{map[string]any{"text": "from plate"}}}}},
	}
	results, err := c.Batch(context.Background(), ops)
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
	if _, err := c.MarkdownToPlate(context.Background(), "", nil); err != nil {
		t.Fatalf("empty md: %v", err)
	}
	if _, err := c.PlateToMarkdown(context.Background(), nil, nil); err != nil {
		t.Fatalf("nil plate: %v", err)
	}
}

func TestOneShot(t *testing.T) {
	v, err := pure.MarkdownToPlate(context.Background(), "# hi\n")
	if err != nil {
		t.Fatalf("MarkdownToPlate: %v", err)
	}
	if len(v) != 1 || v[0]["type"] != "h1" {
		t.Errorf("unexpected value: %+v", v)
	}
	md, err := pure.PlateToMarkdown(context.Background(), []pure.Node{
		{"type": "h2", "children": []any{map[string]any{"text": "sub"}}},
	})
	if err != nil {
		t.Fatalf("PlateToMarkdown: %v", err)
	}
	if !strings.Contains(md, "## sub") {
		t.Errorf("unexpected md: %q", md)
	}
}

func TestTable_RoundTrip(t *testing.T) {
	c := newConverter(t)
	md := "| Lang | Year |\n| ---- | ---- |\n| Go   | 2009 |\n| Rust | 2010 |\n"
	value, err := c.MarkdownToPlate(context.Background(), md, nil)
	if err != nil {
		t.Fatalf("md->plate: %v", err)
	}
	if len(value) != 1 || value[0]["type"] != "table" {
		t.Fatalf("expected single table node, got %+v", value)
	}
	rows, _ := value[0]["children"].([]any)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (header + 2 body), got %d: %+v", len(rows), rows)
	}
	header, _ := rows[0].(map[string]any)
	if header["type"] != "tr" {
		t.Errorf("rows[0].type = %v, want tr", header["type"])
	}
	headerCells, _ := header["children"].([]any)
	firstHeader, _ := headerCells[0].(map[string]any)
	if firstHeader["type"] != "th" {
		t.Errorf("first header cell type = %v, want th", firstHeader["type"])
	}
	out, err := c.PlateToMarkdown(context.Background(), value, nil)
	if err != nil {
		t.Fatalf("plate->md: %v", err)
	}
	for _, want := range []string{"| Lang", "| Year", "| Go", "| Rust"} {
		if !strings.Contains(out, want) {
			t.Errorf("serialized markdown missing %q: %s", want, out)
		}
	}
}

func TestDisableOption(t *testing.T) {
	c := newConverter(t)
	withLists, err := c.MarkdownToPlate(context.Background(), "- one\n- two\n", nil)
	if err != nil {
		t.Fatalf("with lists: %v", err)
	}
	withoutLists, err := c.MarkdownToPlate(context.Background(), "- one\n- two\n",
		&pure.Options{Disable: []string{"lists"}})
	if err != nil {
		t.Fatalf("without lists: %v", err)
	}
	t.Logf("with lists:    %+v", withLists)
	t.Logf("without lists: %+v", withoutLists)
	if len(withLists) > 0 && len(withoutLists) > 0 &&
		withLists[0]["type"] == withoutLists[0]["type"] {
		t.Errorf("expected different top-level types between enabled / disabled lists; both were %v",
			withLists[0]["type"])
	}
}

// TestFixtures compares md->plate output against the JSON fixtures
// captured from the WASM backend. The check is structural (decoded
// equality after re-marshaling) so map ordering doesn't matter.
func TestFixtures(t *testing.T) {
	c := newConverter(t)
	cases := []string{
		"01-formatting",
		"03-link-quote-code",
		"04-image-and-rule",
		"05-table",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			mdPath := filepath.Join("..", "examples", name+".md")
			jsonPath := filepath.Join("..", "examples", name+".json")
			md, err := os.ReadFile(mdPath)
			if err != nil {
				t.Fatalf("read md: %v", err)
			}
			wantBytes, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("read json: %v", err)
			}
			var want []any
			if err := json.Unmarshal(wantBytes, &want); err != nil {
				t.Fatalf("decode want: %v", err)
			}
			got, err := c.MarkdownToPlate(context.Background(), string(md), nil)
			if err != nil {
				t.Fatalf("md->plate: %v", err)
			}
			// Normalize through JSON to compare without map-iteration noise.
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			var gotN, wantN any
			_ = json.Unmarshal(gotJSON, &gotN)
			_ = json.Unmarshal(wantJSON, &wantN)
			if !jsonDeepEqual(gotN, wantN) {
				t.Errorf("fixture mismatch\n got: %s\nwant: %s", indentJSON(gotJSON), indentJSON(wantJSON))
			}
		})
	}
}

// TestListsFixture is split out because the WASM `02-lists.json` uses
// `listStart` only on ordered items; verify we match that.
func TestListsFixture(t *testing.T) {
	c := newConverter(t)
	mdPath := filepath.Join("..", "examples", "02-lists.md")
	jsonPath := filepath.Join("..", "examples", "02-lists.json")
	md, _ := os.ReadFile(mdPath)
	wantBytes, _ := os.ReadFile(jsonPath)
	var want []any
	_ = json.Unmarshal(wantBytes, &want)
	got, err := c.MarkdownToPlate(context.Background(), string(md), nil)
	if err != nil {
		t.Fatalf("md->plate: %v", err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	var gotN, wantN any
	_ = json.Unmarshal(gotJSON, &gotN)
	_ = json.Unmarshal(wantJSON, &wantN)
	if !jsonDeepEqual(gotN, wantN) {
		t.Errorf("fixture mismatch\n got: %s\nwant: %s", indentJSON(gotJSON), indentJSON(wantJSON))
	}
}

func indentJSON(b []byte) string {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}

// jsonDeepEqual compares two values that came out of json.Unmarshal into
// `any`. Map keys with the same content compare equal regardless of
// insertion order.
func jsonDeepEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !jsonDeepEqual(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonDeepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

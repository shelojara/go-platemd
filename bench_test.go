package platemd_test

import (
	"context"
	"strings"
	"testing"

	platemd "github.com/shelojara/go-platemd-wasm"
)

const smallDoc = "# Hello\n\nSome **bold** text and a [link](https://example.com).\n"

// mediumDoc is a few KB of mixed markdown — roughly an article-sized doc.
var mediumDoc = buildMediumDoc()

func buildMediumDoc() string {
	var b strings.Builder
	for i := 0; i < 10; i++ {
		b.WriteString("# Section\n\nIntro paragraph with **bold** and _italic_ and `code`.\n\n")
		b.WriteString("- first item\n- second item with [a link](https://example.com)\n- third item\n\n")
		b.WriteString("> A quoted line that runs on for a while.\n\n")
		b.WriteString("```go\nfunc main() { fmt.Println(\"hi\") }\n```\n\n")
	}
	return b.String()
}

// One-shot path: fresh WASM instance per call. Pays JS engine boot every time.
func BenchmarkConverter_MdToPlate_Small(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.MarkdownToPlate(ctx, smallDoc, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConverter_PlateToMd_Small(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	value, err := c.MarkdownToPlate(ctx, smallDoc, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.PlateToMarkdown(ctx, value, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// Worker path: one persistent WASM instance, default editor cached at JS load.
func BenchmarkWorker_MdToPlate_Small(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	w, err := c.NewWorker(ctx)
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.MarkdownToPlate(ctx, smallDoc, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWorker_PlateToMd_Small(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	w, err := c.NewWorker(ctx)
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()
	value, err := w.MarkdownToPlate(ctx, smallDoc, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.PlateToMarkdown(ctx, value, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWorker_MdToPlate_Medium(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	w, err := c.NewWorker(ctx)
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.MarkdownToPlate(ctx, mediumDoc, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// Batch on a fresh Converter: one engine boot amortized across N ops.
func BenchmarkConverter_Batch10(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	ops := make([]platemd.BatchOp, 10)
	for i := range ops {
		ops[i] = platemd.BatchOp{Markdown: smallDoc}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Batch(ctx, ops); err != nil {
			b.Fatal(err)
		}
	}
}

// Batch on a warm Worker: pure per-op cost (no engine boot at all).
func BenchmarkWorker_Batch10(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	w, err := c.NewWorker(ctx)
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()
	ops := make([]platemd.BatchOp, 10)
	for i := range ops {
		ops[i] = platemd.BatchOp{Markdown: smallDoc}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.Batch(ctx, ops); err != nil {
			b.Fatal(err)
		}
	}
}

// Plate→md batch: serialize is the more expensive direction, this shows
// how it amortizes inside a single batch round-trip.
func BenchmarkWorker_BatchPlateToMd10(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	w, err := c.NewWorker(ctx)
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()
	value, err := w.MarkdownToPlate(ctx, smallDoc, nil)
	if err != nil {
		b.Fatal(err)
	}
	ops := make([]platemd.BatchOp, 10)
	for i := range ops {
		ops[i] = platemd.BatchOp{PlateValue: value}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.Batch(ctx, ops); err != nil {
			b.Fatal(err)
		}
	}
}

// One-time costs.
func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c, err := platemd.New()
		if err != nil {
			b.Fatal(err)
		}
		_ = c.Close()
	}
}

func BenchmarkNewWorker(b *testing.B) {
	c, err := platemd.New()
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w, err := c.NewWorker(ctx)
		if err != nil {
			b.Fatal(err)
		}
		_ = w.Close()
	}
}

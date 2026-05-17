package platemd_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	platemd "github.com/shelojara/go-platemd-wasm"
)

func newWorker(t *testing.T) (*platemd.Converter, *platemd.Worker) {
	t.Helper()
	c := newConverter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	w, err := c.NewWorker(ctx)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return c, w
}

func TestWorker_RoundTrip(t *testing.T) {
	_, w := newWorker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	src := "# Title\n\n- one\n- two\n\nA [link](https://example.com) and `code`.\n"
	value, err := w.MarkdownToPlate(ctx, src, nil)
	if err != nil {
		t.Fatalf("md->plate: %v", err)
	}
	if len(value) == 0 {
		t.Fatal("empty plate value")
	}
	md, err := w.PlateToMarkdown(ctx, value, nil)
	if err != nil {
		t.Fatalf("plate->md: %v", err)
	}
	if !strings.Contains(md, "# Title") || !strings.Contains(md, "[link]") {
		t.Errorf("round-trip lost content: %q", md)
	}
}

func TestWorker_RepeatedCalls(t *testing.T) {
	_, w := newWorker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 20 calls on the same worker — each should be fast and not leak
	// state between calls.
	for i := 0; i < 20; i++ {
		v, err := w.MarkdownToPlate(ctx, "# Hello\n", nil)
		if err != nil {
			t.Fatalf("iter %d md->plate: %v", i, err)
		}
		if got, want := v[0]["type"], "h1"; got != want {
			t.Fatalf("iter %d: top-level type = %v, want %v", i, got, want)
		}
		md, err := w.PlateToMarkdown(ctx, v, nil)
		if err != nil {
			t.Fatalf("iter %d plate->md: %v", i, err)
		}
		if !strings.Contains(md, "# Hello") {
			t.Fatalf("iter %d lost heading: %q", i, md)
		}
	}
}

func TestWorker_Batch(t *testing.T) {
	_, w := newWorker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := w.Batch(ctx, []platemd.BatchOp{
		{Markdown: "# A\n"},
		{Markdown: "## B\n"},
		{PlateValue: []platemd.Node{{"type": "p", "children": []any{map[string]any{"text": "x"}}}}},
	})
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].PlateValue[0]["type"] != "h1" {
		t.Errorf("result[0]: %+v", results[0])
	}
	if !strings.Contains(results[2].Markdown, "x") {
		t.Errorf("result[2]: %+v", results[2])
	}
}

func TestWorker_ErrorRecovery(t *testing.T) {
	_, w := newWorker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Send something the JS will reject (a Plate value with a totally
	// unknown shape that breaks serialize). Then verify the next call
	// still works — worker state should survive a per-op error.
	_, _ = w.PlateToMarkdown(ctx, []platemd.Node{{"weird": "no type or children"}}, nil)
	// Whether that errored or returned "" doesn't matter; the worker
	// should still be usable.

	v, err := w.MarkdownToPlate(ctx, "# After error\n", nil)
	if err != nil {
		t.Fatalf("MarkdownToPlate after probable error: %v", err)
	}
	if v[0]["type"] != "h1" {
		t.Errorf("worker corrupted: %+v", v)
	}
}

func TestWorker_ConcurrentSerialized(t *testing.T) {
	_, w := newWorker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// The Worker serializes via mutex — concurrent callers shouldn't
	// corrupt each other, just queue up.
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := w.MarkdownToPlate(ctx, "# Concurrent\n", nil)
			if err != nil {
				errs <- err
				return
			}
			if v[0]["type"] != "h1" {
				errs <- nil // sentinel for "wrong content"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Error(e)
		} else {
			t.Error("wrong content from concurrent caller")
		}
	}
}

func TestWorker_CloseDuringIdle(t *testing.T) {
	c := newConverter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	w, err := c.NewWorker(ctx)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	// Use the worker briefly.
	if _, err := w.MarkdownToPlate(ctx, "# x\n", nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// Calls after Close should fail rather than hang.
	done := make(chan error, 1)
	go func() {
		_, err := w.MarkdownToPlate(ctx, "# y\n", nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("call after Close succeeded; expected failure")
		}
	case <-time.After(5 * time.Second):
		t.Error("call after Close hung")
	}
}

// Benchmarks live in bench_test.go.

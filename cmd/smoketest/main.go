// Smoke-test harness: opens a Worker against the embedded WASM and
// sends a payload (default: a ping op, override via CLI args). Prints
// the response and how long each phase took. Useful for poking at the
// WASM blob without writing a Go test.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	platemd "github.com/shelojara/go-platemd-wasm"
)

func main() {
	t0 := time.Now()
	c, err := platemd.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "New:", err)
		os.Exit(1)
	}
	defer c.Close()
	fmt.Fprintf(os.Stderr, "compile: %v\n", time.Since(t0))

	ctx := context.Background()
	t1 := time.Now()
	w, err := c.NewWorker(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NewWorker:", err)
		os.Exit(1)
	}
	defer w.Close()
	fmt.Fprintf(os.Stderr, "worker boot: %v\n", time.Since(t1))

	payload := "# Hello\n\nThis is **bold** text.\n\n- one\n- two\n"
	if len(os.Args) > 1 {
		payload = strings.Join(os.Args[1:], " ")
	}

	t2 := time.Now()
	value, err := w.MarkdownToPlate(ctx, payload, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "MarkdownToPlate:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "md->plate: %v\n", time.Since(t2))

	t3 := time.Now()
	md, err := w.PlateToMarkdown(ctx, value, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "PlateToMarkdown:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "plate->md: %v\n", time.Since(t3))

	fmt.Printf("in:\n%s\n", payload)
	fmt.Printf("\nout (md->plate, %d nodes):\n%+v\n", len(value), value)
	fmt.Printf("\nout (plate->md):\n%s\n", md)
}

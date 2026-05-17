// Smoke-test harness: instantiates the embedded WASM with a hard-coded
// stdin payload and prints whatever it writes to stdout / stderr.
// Not part of the library surface; lives under cmd/ for local debugging.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	platemd "github.com/shelojara/go-platemd-wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	ctx := context.Background()

	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(true))
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	t0 := time.Now()
	compiled, err := r.CompileModule(ctx, platemd.WasmBlob())
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "compile: %v\n", time.Since(t0))

	payload := `{"op":"ping"}`
	if len(os.Args) > 1 {
		payload = strings.Join(os.Args[1:], " ")
	}

	stdin := bytes.NewBufferString(payload)
	var stdout, stderr bytes.Buffer

	cfg := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs("plate.wasm")

	t1 := time.Now()
	mod, err := r.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stderr=%s\n", stderr.String())
		fmt.Fprintf(os.Stderr, "stdout=%s\n", stdout.String())
		fmt.Fprintf(os.Stderr, "instantiate: %v\n", err)
		os.Exit(1)
	}
	mod.Close(ctx)
	fmt.Fprintf(os.Stderr, "run: %v\n", time.Since(t1))
	fmt.Fprintf(os.Stderr, "stderr: %s\n", stderr.String())
	fmt.Printf("%s\n", stdout.String())
}

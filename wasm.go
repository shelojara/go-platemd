package platemd

import _ "embed"

//go:embed internal/wasm/plate.wasm
var wasmBlob []byte

// WasmBlob returns the embedded WASM module bytes. Primarily useful for
// tooling and tests; library consumers should call New().
func WasmBlob() []byte { return wasmBlob }

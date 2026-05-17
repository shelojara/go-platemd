JAVY ?= javy
JS_DIR := js
WASM_OUT := internal/wasm/plate.wasm

.PHONY: help wasm js-install bundle test clean

help:
	@echo "Targets:"
	@echo "  make wasm        rebuild internal/wasm/plate.wasm from js/ sources"
	@echo "  make js-install  npm install JS dependencies"
	@echo "  make bundle      bundle js/src into a single file (no WASM compile)"
	@echo "  make test        run Go tests"
	@echo "  make clean       remove generated bundle artifacts"
	@echo ""
	@echo "Requires: node, npm, javy (https://github.com/bytecodealliance/javy/releases)"
	@echo "JAVY=/path/to/javy make wasm   # if javy is not on PATH"

js-install:
	cd $(JS_DIR) && npm install --no-audit --no-fund

bundle:
	cd $(JS_DIR) && npm run bundle

wasm: bundle
	cd $(JS_DIR) && $(JAVY) build -C dynamic=n -C source=omitted -J event-loop=y -J text-encoding=y -o ../$(WASM_OUT) dist/bundle.js
	@echo ""
	@echo "Built $(WASM_OUT) ($$(wc -c < $(WASM_OUT)) bytes)"

test:
	go test ./... -count=1

clean:
	rm -rf $(JS_DIR)/dist

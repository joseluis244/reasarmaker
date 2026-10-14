WASM_DIR   := webapp
WASM       := $(WASM_DIR)/public/sar.wasm
WASM_EXEC  := $(WASM_DIR)/src/wasm/wasm_exec.js

.PHONY: all wasm test smoke e2e web-build dev clean

## wasm: build the Go engine as a WebAssembly module + wasm_exec.js glue
wasm:
	mkdir -p $(WASM_DIR)/public $(WASM_DIR)/src/wasm
	GOOS=js GOARCH=wasm go build -o $(WASM) ./wasm
	rm -f $(WASM_EXEC)
	cp "$(shell go env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_EXEC) && chmod +w $(WASM_EXEC)

## test: run the Go engine tests
test:
	go test ./...

## smoke: differential test, native Go engine vs the wasm module
smoke: wasm
	node wasm/smoke.mjs

## e2e: drive the app in a real (headless) Chrome
e2e: wasm
	cd $(WASM_DIR) && npm run e2e

## web-build: production build of the React app (includes the wasm)
web-build: wasm
	cd $(WASM_DIR) && npm run build

## dev: run the Vite dev server
dev: wasm
	cd $(WASM_DIR) && npm run dev

clean:
	rm -f $(WASM) $(WASM_EXEC)
	rm -rf $(WASM_DIR)/dist

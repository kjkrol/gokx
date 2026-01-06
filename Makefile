GO       = go
GOROOT  := $(shell go env GOROOT)

DEMO ?= cmd/sim-demo
DEMO_DIR := $(patsubst %/,%,$(dir $(DEMO)))
GO_DEMO := $(if $(filter %.go,$(DEMO)),$(DEMO_DIR),$(DEMO))
GO_DEMO_PKG := ./$(GO_DEMO)
WASM_DIR  = cmd/wasm-demo

BIN_X11   = bin/demo-x11
BIN_SDL2  = bin/demo-sdl2

# ------------------- RUN -------------------

run-x11:
	$(GO) run -tags x11 $(GO_DEMO_PKG)

run-sdl2:
	$(GO) run $(GO_DEMO_PKG)

# ------------------- BUILD -----------------

build-x11:
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 $(GO) build -tags x11 -o $(BIN_X11) $(GO_DEMO_PKG)

build-sdl2:
	@mkdir -p bin
	$(GO) build -o $(BIN_SDL2) $(GO_DEMO_PKG)

wasm-build:
	cp $(GOROOT)/lib/wasm/wasm_exec.js $(WASM_DIR)/
	GOOS=js GOARCH=wasm $(GO) build -o $(WASM_DIR)/main.wasm $(GO_DEMO_PKG)
	cd $(WASM_DIR) && go build -o server server.go

# ------------------- SERVE -----------------
wasm-serve: wasm-build
	$(WASM_DIR)/server

# ------------------- CLEAN -----------------

clear:
	rm -rf bin/
	rm -f $(WASM_DIR)/main.wasm
	rm -f $(WASM_DIR)/wasm_exec.js
	rm -f $(WASM_DIR)/server.log
	rm -f $(WASM_DIR)/server

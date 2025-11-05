# GOKX

*aka "Golang kjkrol eXperiments"*

**GOKX** is a Go library that provides a lightweight experimental framework for 2D graphics applications.  

It includes:

- a **platform layer** (`internal/platform`) with support for multiple backends:
  - SDL2 (default, multiplatform)
  - Linux X11
  - WebAssembly (WASM)
- a **graphics layer** (`pkg/gfx`) with abstractions for:
  - windows
  - panes and layered panels
  - animations
  - event handling
- example demo applications (`cmd/`):
  - a basic SDL2/X11 demo
  - a quadtree visualization demo
  - a WebAssembly demo running in the browser

The project is intended as a foundation for experimenting with graphics, input events, and rendering across different environments in pure Go.

## Quick Example

A minimal example using `gfx.Window`:

```go
package main

import (
	"log"

	"github.com/kjkrol/gokx/pkg/gfx"
)

func main() {
	win, err := gfx.NewWindow("GOKX Demo", 800, 600)
	if err != nil {
		log.Fatalf("failed to create window: %v", err)
	}
	defer win.Close()

	for win.IsOpen() {
		win.PollEvents()
		win.Clear()
		// TODO: draw objects here
		win.Display()
	}
}
```

## Build

```sh
go build -o gokx ./cmd
# or with X11 wrapper
go build -tags x11 -o gokx ./cmd
# or with make
make build-x11
# or
make build-sdl2
```

### Prerequisites

#### Default SDL2 (multiplatform) backend

- **Linux dependencies**

```sh
sudo apt update
sudo apt install libsdl2-dev
```

- **macOS dependencies**

```sh
brew install sdl2
pkg-config --modversion sdl2
```

Run example:

```sh
go run cmd/main.go
# or
make run-sdl2
```

#### Linux X11 backend

```sh
sudo apt update
sudo apt install libx11-dev
```

Run example:

```sh
go run -tags x11 cmd/main.go
# or
make run-x11
```

#### WASM backend

To run the WebAssembly demo:

```sh
make wasm-serve
```

## Other demos

You can select which demo app to build/run using `make`. For example:

```sh
make DEMO=cmd/demo-quadtree/main.go run-sdl2
```

## License

MIT

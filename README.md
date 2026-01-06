# GOKX

*aka "Golang kjkrol eXperiments"*

**GOKX** is a Go library that provides a lightweight experimental framework for 2D graphics applications.  

It includes:

- a **platform layer** (`internal/platform`) with support for multiple backends:
  - SDL2 (OpenGL 3.3 core)
  - WebAssembly (WebGL2)
  - Linux X11 (legacy, not part of the GPU-only path)
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

`internal/renderer.NewRendererFactory` expects a single-source shader (stage/pass defines) and returns a renderer factory for `gfx.NewWindow`.
See `cmd/sim-demo/main.go` and `cmd/sim-demo/shader.glsl` for a complete example.

```go
package main

import (
	_ "embed"

	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokg/pkg/spatial"
	"github.com/kjkrol/gokx/internal/renderer"
	"github.com/kjkrol/gokx/pkg/gfx"
	"github.com/kjkrol/gokx/pkg/grid"
	"github.com/kjkrol/gokx/pkg/gridbridge"
)

//go:embed shader.glsl
var shaderSource string

func main() {
	worldRes := spatial.Size1024x1024
	bridge := gridbridge.NewBridge()
	win := gfx.NewWindow(gfx.WindowConfig{
		Width:  800,
		Height: 600,
		Title:  "GOKX Demo",
		World: gfx.WorldConfig{
			WorldResolution: worldRes,
			WorldWrap:       true,
		},
	}, renderer.NewRendererFactory(renderer.RendererConfig{ShaderSource: shaderSource}, bridge))
	defer win.Close()

	pane := win.GetDefaultPane()
	space := plane.NewToroidal2D(int(worldRes.Side()), int(worldRes.Side()))
	manager := grid.NewMultiBucketGridManager(space, worldRes, 2, spatial.Size64x64, 16)
	bridge.AttachPane(pane, manager)

	win.Show()
	win.RefreshRate(60)
	win.ListenEvents(func(event gfx.Event) {})
}
```

## Build

```sh
go build -o gokx ./cmd
# or with make
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
go run ./cmd/sim-demo
# or
make run-sdl2
```

#### WASM backend

To run the WebAssembly demo:

```sh
make wasm-serve
```

## Other demos

You can select which demo app to build/run using `make`. For example:

```sh
make DEMO=cmd/demo-quadtree run-sdl2
```

## License

MIT

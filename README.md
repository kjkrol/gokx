# GOKX

## Build

```shell
go build -o gokx ./cmd
# or with x11 wrapper
go build -tags x11 -o gokx ./cmd
```

### Prerequisits

#### Default multiplatform SDL2 version

- Linux dependencies

```shell
sudo apt update
sudo apt install libsdl2-dev
```

- OSX dependencies

```shell
brew install sdl2
pkg-config --modversion sdl2
```

Run example

```shell
go run cmd/main.go
```

#### Linux X11 version

```shell
sudo apt update
sudo apt install libx11-dev
```

Run example

```shell
go run -tags x11 cmd/main.go 
```


#### WASM

cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ./wasm_exec.js

GOOS=js GOARCH=wasm go build -o main.wasm cmd/main.go

python3 -m http.server 8080

http://localhost:8080/index.html




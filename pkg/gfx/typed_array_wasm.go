//go:build js && wasm

package gfx

import (
	"syscall/js"
	"unsafe"
)

func float32Array(data []float32) js.Value {
	arr := js.Global().Get("Float32Array").New(len(data))
	if len(data) == 0 {
		return arr
	}
	buf := arr.Get("buffer")
	view := js.Global().Get("Uint8Array").New(buf, arr.Get("byteOffset"), arr.Get("byteLength"))
	js.CopyBytesToJS(view, float32Bytes(data))
	return arr
}

func float32Bytes(data []float32) []byte {
	if len(data) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*4)
}

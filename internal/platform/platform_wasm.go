//go:build js && wasm

package platform

import (
	"fmt"
	"image"
	"syscall/js"
	"time"
)

type wasmWindowWrapper struct {
	canvas js.Value
	ctx    js.Value
	events chan Event
	conf   WindowConfig
	closed bool
}

func NewPlatformWindowWrapper(conf WindowConfig) PlatformWindowWrapper {
	doc := js.Global().Get("document")

	doc.Set("title", conf.Title)

	canvas := doc.Call("createElement", "canvas")
	canvas.Set("width", conf.Width)
	canvas.Set("height", conf.Height)
	canvas.Get("style").Set("border", fmt.Sprintf("%dpx solid black", conf.BorderWidth))
	canvas.Get("style").Set("position", "absolute")
	canvas.Get("style").Set("left", fmt.Sprintf("%dpx", conf.PositionX))
	canvas.Get("style").Set("top", fmt.Sprintf("%dpx", conf.PositionY))

	doc.Get("body").Call("appendChild", canvas)

	ctx := canvas.Call("getContext", "2d")

	w := &wasmWindowWrapper{
		canvas: canvas,
		ctx:    ctx,
		events: make(chan Event, 64),
		conf:   conf,
	}

	// event listeners
	addEventListener := func(target js.Value, event string, f func(js.Value)) {
		target.Call("addEventListener", event, js.FuncOf(func(this js.Value, args []js.Value) any {
			f(args[0])
			return nil
		}))
	}

	addEventListener(doc, "keydown", func(e js.Value) {
		key := e.Get("key").String()
		w.events <- KeyPress{Code: 0, Label: key}
	})
	addEventListener(doc, "keyup", func(e js.Value) {
		key := e.Get("key").String()
		w.events <- KeyRelease{Code: 0, Label: key}
	})
	addEventListener(canvas, "mousedown", func(e js.Value) {
		w.events <- ButtonPress{
			Button: uint32(e.Get("button").Int()),
			X:      e.Get("offsetX").Int(),
			Y:      e.Get("offsetY").Int(),
		}
	})
	addEventListener(canvas, "mouseup", func(e js.Value) {
		w.events <- ButtonRelease{
			Button: uint32(e.Get("button").Int()),
			X:      e.Get("offsetX").Int(),
			Y:      e.Get("offsetY").Int(),
		}
	})
	addEventListener(canvas, "mousemove", func(e js.Value) {
		w.events <- MotionNotify{
			X: e.Get("offsetX").Int(),
			Y: e.Get("offsetY").Int(),
		}
	})

	// emit "create window" event
	go func() {
		time.Sleep(10 * time.Millisecond)
		w.events <- CreateNotify{}
	}()

	return w
}

func (w *wasmWindowWrapper) Show() {
	// pierwsza ramka (czarne tło)
	w.ctx.Call("fillRect", 0, 0, w.canvas.Get("width").Int(), w.canvas.Get("height").Int())
}

func (w *wasmWindowWrapper) Close() {
	if !w.closed {
		w.closed = true
		w.events <- DestroyNotify{}
	}
}

func (w *wasmWindowWrapper) NextEventTimeout(timeoutMs int) Event {
	select {
	case e := <-w.events:
		return e
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return TimeoutEvent{}
	}
}

func (w *wasmWindowWrapper) NewPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) PlatformImageWrapper {
	return &wasmImageWrapper{parent: w, img: img, offsetX: offsetX, offsetY: offsetY}
}

// ---------------- IMAGE ----------------

type wasmImageWrapper struct {
	parent  *wasmWindowWrapper
	img     *image.RGBA
	offsetX int
	offsetY int
}

func (i *wasmImageWrapper) Update(rect image.Rectangle) {
	if rect.Empty() {
		return
	}

	w := i.img.Rect.Dx()
	h := i.img.Rect.Dy()

	uint8Array := js.Global().Get("Uint8ClampedArray").New(len(i.img.Pix))
	js.CopyBytesToJS(uint8Array, i.img.Pix)
	imageData := js.Global().Get("ImageData").New(uint8Array, w, h)

	i.parent.ctx.Call("putImageData", imageData, i.offsetX, i.offsetY)
}

func (i *wasmImageWrapper) Delete() {
	// nic do sprzątania, GC załatwi
}

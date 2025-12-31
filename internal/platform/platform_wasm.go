//go:build js && wasm

package platform

import (
	"fmt"
	"syscall/js"
	"time"
)

type wasmWindowWrapper struct {
	canvas js.Value
	gl     js.Value
	events chan Event
	conf   WindowConfig
	closed bool

	funcs   []js.Func
	removes []struct {
		target js.Value
		typ    string
		fn     js.Func
	}
}

func NewPlatformWindowWrapper(conf WindowConfig) PlatformWindowWrapper {
	doc := js.Global().Get("document")
	doc.Set("title", conf.Title)

	canvas := doc.Call("createElement", "canvas")
	canvas.Set("width", conf.Width)
	canvas.Set("height", conf.Height)
	style := canvas.Get("style")
	style.Set("border", fmt.Sprintf("%dpx solid black", conf.BorderWidth))
	style.Set("position", "absolute")
	style.Set("left", fmt.Sprintf("%dpx", conf.PositionX))
	style.Set("top", fmt.Sprintf("%dpx", conf.PositionY))
	style.Set("pointerEvents", "auto")

	// umożliw focus
	canvas.Call("setAttribute", "tabindex", "0")

	doc.Get("body").Call("appendChild", canvas)

	gl := canvas.Call("getContext", "webgl2")
	if gl.IsNull() || gl.IsUndefined() {
		panic("webgl2 context is required")
	}

	w := &wasmWindowWrapper{
		canvas: canvas,
		gl:     gl,
		events: make(chan Event, 64),
		conf:   conf,
	}

	// helper do listenerów
	addEventListener := func(target js.Value, event string, f func(js.Value)) {
		fn := js.FuncOf(func(this js.Value, args []js.Value) any {
			if len(args) == 0 {
				return nil
			}
			e := args[0]
			e.Call("preventDefault")
			e.Call("stopPropagation")
			f(e)
			return nil
		})
		target.Call("addEventListener", event, fn)
		w.funcs = append(w.funcs, fn)
		w.removes = append(w.removes, struct {
			target js.Value
			typ    string
			fn     js.Func
		}{target: target, typ: event, fn: fn})
	}

	// pomocnik: współrzędne względem canvasa
	getCanvasCoords := func(e js.Value) (int, int) {
		rect := w.canvas.Call("getBoundingClientRect")
		cw := float64(w.canvas.Get("width").Int())
		ch := float64(w.canvas.Get("height").Int())
		rw := rect.Get("width").Float()
		rh := rect.Get("height").Float()

		scaleX := 1.0
		scaleY := 1.0
		if rw != 0 {
			scaleX = cw / rw
		}
		if rh != 0 {
			scaleY = ch / rh
		}

		clientX := e.Get("clientX").Float()
		clientY := e.Get("clientY").Float()
		x := (clientX - rect.Get("left").Float()) * scaleX
		y := (clientY - rect.Get("top").Float()) * scaleY
		return int(x + 0.5), int(y + 0.5)
	}

	// klawiatura
	addEventListener(doc, "keydown", func(e js.Value) {
		key := e.Get("key").String()
		w.events <- KeyPress{Code: 0, Label: key}
	})
	addEventListener(doc, "keyup", func(e js.Value) {
		key := e.Get("key").String()
		w.events <- KeyRelease{Code: 0, Label: key}
	})

	// mapowanie DOM -> SDL/X11 (0,1,2) -> (1,2,3)
	mapMouseButton := func(e js.Value) uint32 {
		switch e.Get("button").Int() {
		case 0:
			return 1 // left
		case 1:
			return 2 // middle
		case 2:
			return 3 // right
		default:
			// na wszelki wypadek: przesuwamy o +1
			return uint32(e.Get("button").Int() + 1)
		}
	}

	// mysz
	addEventListener(canvas, "mousedown", func(e js.Value) {
		x, y := getCanvasCoords(e)
		w.events <- ButtonPress{
			Button: mapMouseButton(e), // <— TU
			X:      x,
			Y:      y,
		}
	})

	addEventListener(doc, "mouseup", func(e js.Value) {
		x, y := getCanvasCoords(e)
		w.events <- ButtonRelease{
			Button: mapMouseButton(e), // <— TU
			X:      x,
			Y:      y,
		}
	})

	addEventListener(canvas, "pointermove", func(e js.Value) {
		coalesced := e.Call("getCoalescedEvents")
		length := coalesced.Get("length").Int()
		if length == 0 {
			x, y := getCanvasCoords(e)
			w.events <- MotionNotify{X: x, Y: y}
			return
		}
		for i := 0; i < length; i++ {
			ev := coalesced.Index(i)
			x, y := getCanvasCoords(ev)
			w.events <- MotionNotify{X: x, Y: y}
		}
	})

	addEventListener(canvas, "wheel", func(e js.Value) {
		deltaX := -e.Get("deltaX").Float()
		deltaY := -e.Get("deltaY").Float()
		if e.Get("deltaMode").Int() == 0 {
			deltaX /= 120.0
			deltaY /= 120.0
		}
		x, y := getCanvasCoords(e)
		w.events <- MouseWheel{
			DeltaX: deltaX,
			DeltaY: deltaY,
			X:      x,
			Y:      y,
		}
	})

	// wyłącz menu kontekstowe
	addEventListener(canvas, "contextmenu", func(e js.Value) {})

	// fokus i CreateNotify
	go func() {
		time.Sleep(10 * time.Millisecond)
		canvas.Call("focus")
		w.events <- CreateNotify{}
	}()

	return w
}

func (w *wasmWindowWrapper) Show() {
	if w.gl.IsUndefined() || w.gl.IsNull() {
		return
	}
	w.gl.Call("clearColor", 0, 0, 0, 1)
	w.gl.Call("clear", w.gl.Get("COLOR_BUFFER_BIT"))
}

func (w *wasmWindowWrapper) Close() {
	if w.closed {
		return
	}
	w.closed = true

	// usuń listenery
	for _, r := range w.removes {
		r.target.Call("removeEventListener", r.typ, r.fn)
	}
	for i := range w.funcs {
		w.funcs[i].Release()
	}
	w.funcs = nil
	w.removes = nil

	w.events <- DestroyNotify{}
}

func (w *wasmWindowWrapper) NextEventTimeout(timeoutMs int) Event {
	select {
	case e := <-w.events:
		return e
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return TimeoutEvent{}
	}
}

func (w *wasmWindowWrapper) BeginFrame() {}
func (w *wasmWindowWrapper) EndFrame()   {}

func (w *wasmWindowWrapper) GLContext() any {
	return w.gl
}

//go:build darwin && !x11 && cgo && gpu

package platform

/*
#cgo pkg-config: sdl2
#include <stdlib.h>
#include <SDL2/SDL.h>
static inline void my_SDL_DestroyTexture(SDL_Texture* t) {
	if (t) {
		SDL_DestroyTexture(t);
	}
}
*/
import "C"

import (
	"fmt"
	"image"
	"runtime"
	"unsafe"
)

type sdlGPUWindowWrapper struct {
	window   *C.SDL_Window
	renderer *C.SDL_Renderer
	title    string
	width    int
	height   int
}

func NewPlatformWindowWrapper(conf WindowConfig) PlatformWindowWrapper {
	runtime.LockOSThread()

	metal := C.CString("metal")
	C.SDL_SetHint(C.SDL_HINT_RENDER_DRIVER, metal)
	C.free(unsafe.Pointer(metal))

	if C.SDL_Init(C.SDL_INIT_VIDEO) != 0 {
		runtime.UnlockOSThread()
		panic(fmt.Sprintf("SDL_Init error: %s", C.GoString(C.SDL_GetError())))
	}

	cTitle := C.CString(conf.Title)
	defer C.free(unsafe.Pointer(cTitle))

	window := C.SDL_CreateWindow(
		cTitle,
		C.SDL_WINDOWPOS_CENTERED, C.SDL_WINDOWPOS_CENTERED,
		C.int(conf.Width), C.int(conf.Height),
		C.SDL_WINDOW_SHOWN|C.SDL_WINDOW_ALLOW_HIGHDPI,
	)
	if window == nil {
		C.SDL_Quit()
		runtime.UnlockOSThread()
		panic(fmt.Sprintf("SDL_CreateWindow error: %s", C.GoString(C.SDL_GetError())))
	}

	renderer := createDarwinRenderer(window, conf.Width, conf.Height)
	if renderer == nil {
		C.SDL_DestroyWindow(window)
		C.SDL_Quit()
		runtime.UnlockOSThread()
		panic(fmt.Sprintf("SDL_CreateRenderer error: %s", C.GoString(C.SDL_GetError())))
	}

	return &sdlGPUWindowWrapper{
		window:   window,
		renderer: renderer,
		title:    conf.Title,
		width:    conf.Width,
		height:   conf.Height,
	}
}

func createDarwinRenderer(window *C.SDL_Window, width, height int) *C.SDL_Renderer {
	flags := []C.Uint{
		C.SDL_RENDERER_ACCELERATED | C.SDL_RENDERER_PRESENTVSYNC,
		C.SDL_RENDERER_ACCELERATED,
		C.SDL_RENDERER_SOFTWARE,
	}

	for _, flag := range flags {
		r := C.SDL_CreateRenderer(window, -1, flag)
		if r == nil {
			continue
		}
		if rendererWorks(r) {
			C.SDL_RenderSetLogicalSize(r, C.int(width), C.int(height))
			C.SDL_RenderSetIntegerScale(r, C.SDL_TRUE)
			return r
		}
		C.SDL_DestroyRenderer(r)
	}
	return nil
}

func rendererWorks(r *C.SDL_Renderer) bool {
	C.SDL_SetRenderDrawColor(r, 0, 0, 0, 255)
	if C.SDL_RenderClear(r) != 0 {
		return false
	}
	C.SDL_RenderPresent(r)
	return true
}

func (w *sdlGPUWindowWrapper) Show() {
	C.SDL_ShowWindow(w.window)
	C.SDL_EventState(C.SDL_QUIT, C.SDL_ENABLE)

	C.SDL_SetRenderDrawColor(w.renderer, 0, 0, 0, 255)
	C.SDL_RenderClear(w.renderer)
	C.SDL_RenderPresent(w.renderer)
}

func (w *sdlGPUWindowWrapper) Close() {
	C.SDL_DestroyRenderer(w.renderer)
	C.SDL_DestroyWindow(w.window)
	C.SDL_Quit()
	runtime.UnlockOSThread()
}

func (w *sdlGPUWindowWrapper) NextEventTimeout(timeoutMs int) Event {
	var e C.SDL_Event
	if C.SDL_WaitEventTimeout(&e, C.int(timeoutMs)) != 0 {
		return convert(e)
	}
	return TimeoutEvent{}
}

func (w *sdlGPUWindowWrapper) NewPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) PlatformImageWrapper {
	return newSDLGPUImageWrapper(w, img, offsetX, offsetY)
}

type sdlGPUImageWrapper struct {
	window  *sdlGPUWindowWrapper
	texture *C.SDL_Texture
	offsetX int
	offsetY int
	width   int
	height  int
	img     *image.RGBA
}

func newSDLGPUImageWrapper(win *sdlGPUWindowWrapper, img *image.RGBA, offsetX, offsetY int) *sdlGPUImageWrapper {
	texture := C.SDL_CreateTexture(
		win.renderer,
		C.SDL_PIXELFORMAT_RGBA32,
		C.SDL_TEXTUREACCESS_STREAMING,
		C.int(img.Rect.Dx()),
		C.int(img.Rect.Dy()),
	)
	if texture == nil {
		panic(fmt.Sprintf("SDL_CreateTexture error: %s", C.GoString(C.SDL_GetError())))
	}

	return &sdlGPUImageWrapper{
		window:  win,
		texture: texture,
		offsetX: offsetX,
		offsetY: offsetY,
		width:   img.Rect.Dx(),
		height:  img.Rect.Dy(),
		img:     img,
	}
}

func (i *sdlGPUImageWrapper) Update(rect image.Rectangle) {
	if rect.Empty() {
		return
	}

	rect = rect.Intersect(i.img.Rect)
	if rect.Empty() {
		return
	}

	srcX := rect.Min.X - i.img.Rect.Min.X
	srcY := rect.Min.Y - i.img.Rect.Min.Y
	offset := srcY*i.img.Stride + srcX*4

	if offset < 0 || offset >= len(i.img.Pix) {
		return
	}

	srcRect := C.SDL_Rect{
		x: C.int(srcX),
		y: C.int(srcY),
		w: C.int(rect.Dx()),
		h: C.int(rect.Dy()),
	}

	dstRect := C.SDL_Rect{
		x: C.int(rect.Min.X + i.offsetX),
		y: C.int(rect.Min.Y + i.offsetY),
		w: C.int(rect.Dx()),
		h: C.int(rect.Dy()),
	}

	pixels := unsafe.Pointer(&i.img.Pix[offset])
	if C.SDL_UpdateTexture(i.texture, &srcRect, pixels, C.int(i.img.Stride)) != 0 {
		fmt.Println("SDL_UpdateTexture error:", C.GoString(C.SDL_GetError()))
		return
	}

	if C.SDL_RenderCopy(i.window.renderer, i.texture, &srcRect, &dstRect) != 0 {
		fmt.Println("SDL_RenderCopy error:", C.GoString(C.SDL_GetError()))
		return
	}

	C.SDL_RenderPresent(i.window.renderer)

	runtime.KeepAlive(i.window)
	runtime.KeepAlive(i.img)
}

func (i *sdlGPUImageWrapper) Delete() {
	C.my_SDL_DestroyTexture(i.texture)
	i.texture = nil
	i.img = nil
}

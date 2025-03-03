//go:build darwin

package platform

/*
#cgo pkg-config: sdl2
#include <SDL2/SDL.h>
*/
import "C"
import (
	"fmt"
	"image"
	"runtime"
	"unsafe"
)

type sdlWindowWrapper struct {
	window   *C.SDL_Window
	renderer *C.SDL_Renderer
	title    string
	width    int
	height   int
}

func NewPlatformWindowWrapper(conf WindowConfig) PlatformWindowWrapper {
	runtime.LockOSThread()
	if C.SDL_Init(C.SDL_INIT_VIDEO) != 0 {
		panic(fmt.Sprintf("SDL_Init error: %s", C.GoString(C.SDL_GetError())))
	}

	cTitle := C.CString(conf.Title)
	defer C.free(unsafe.Pointer(cTitle))

	window := C.SDL_CreateWindow(cTitle, C.SDL_WINDOWPOS_CENTERED, C.SDL_WINDOWPOS_CENTERED,
		C.int(conf.Width), C.int(conf.Height), C.SDL_WINDOW_SHOWN)
	if window == nil {
		panic(fmt.Sprintf("SDL_CreateWindow error: %s", C.GoString(C.SDL_GetError())))
	}

	renderer := C.SDL_CreateRenderer(window, -1, C.SDL_RENDERER_ACCELERATED)
	if renderer == nil {
		C.SDL_DestroyWindow(window)
		C.SDL_Quit()
		panic(fmt.Sprintf("SDL_CreateRenderer error: %s", C.GoString(C.SDL_GetError())))
	}

	return &sdlWindowWrapper{window, renderer, conf.Title, conf.Width, conf.Height}
}

func (w *sdlWindowWrapper) Show() {
	C.SDL_ShowWindow(w.window)
	C.SDL_EventState(C.SDL_QUIT, C.SDL_ENABLE)
}

func (w *sdlWindowWrapper) Close() {
	C.SDL_DestroyRenderer(w.renderer)
	C.SDL_DestroyWindow(w.window)
	C.SDL_Quit()
}

func (w *sdlWindowWrapper) NextEvent() Event {
	var sdlEvent C.SDL_Event
	if C.SDL_WaitEvent(&sdlEvent) != 0 {
		return convert(sdlEvent)
	}
	return UnknownEvent{}
}

func convert(event C.SDL_Event) Event {
	switch eventType := (*(*C.Uint32)(unsafe.Pointer(&event))); eventType {
	case C.SDL_QUIT:
		return DestroyNotify{}
	case C.SDL_KEYDOWN:
		keyEvent := (*C.SDL_KeyboardEvent)(unsafe.Pointer(&event))
		code := uint64(keyEvent.keysym.scancode)
		label := C.GoString(C.SDL_GetKeyName(keyEvent.keysym.sym))
		return KeyPress{Code: code, Label: label}
	case C.SDL_KEYUP:
		keyEvent := (*C.SDL_KeyboardEvent)(unsafe.Pointer(&event))
		code := uint64(keyEvent.keysym.scancode)
		label := C.GoString(C.SDL_GetKeyName(keyEvent.keysym.sym))
		return KeyRelease{Code: code, Label: label}
	case C.SDL_MOUSEBUTTONDOWN:
		mouseEvent := (*C.SDL_MouseButtonEvent)(unsafe.Pointer(&event))
		return ButtonPress{
			Button: uint32(mouseEvent.button),
			X:      int(mouseEvent.x),
			Y:      int(mouseEvent.y),
		}
	case C.SDL_MOUSEBUTTONUP:
		mouseEvent := (*C.SDL_MouseButtonEvent)(unsafe.Pointer(&event))
		return ButtonRelease{
			Button: uint32(mouseEvent.button),
			X:      int(mouseEvent.x),
			Y:      int(mouseEvent.y),
		}
	case C.SDL_MOUSEMOTION:
		mouseEvent := (*C.SDL_MouseMotionEvent)(unsafe.Pointer(&event))
		return MotionNotify{
			X: int(mouseEvent.x),
			Y: int(mouseEvent.y),
		}
	case C.SDL_WINDOWEVENT:
		windowEvent := (*C.SDL_WindowEvent)(unsafe.Pointer(&event))
		switch windowEvent.event {
		case C.SDL_WINDOWEVENT_EXPOSED:
			return Expose{}
		case C.SDL_WINDOWEVENT_ENTER:
			return EnterNotify{}
		case C.SDL_WINDOWEVENT_LEAVE:
			return LeaveNotify{}
		}
	default:
		fmt.Printf("Unhandled SDL event type: %d\n", eventType)
		return UnknownEvent{}
	}
	return UnexpectedEvent{}
}

func (w *sdlWindowWrapper) NewPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) PlatformImageWrapper {
	return newSDLImageWrapper(w, img, offsetX, offsetY)
}

type sdlImageWrapper struct {
	window  *sdlWindowWrapper
	texture *C.SDL_Texture
	offsetX int
	offsetY int
	width   int
	height  int
	img     *image.RGBA
}

func newSDLImageWrapper(win *sdlWindowWrapper, img *image.RGBA, offsetX, offsetY int) *sdlImageWrapper {
	texture := C.SDL_CreateTexture(win.renderer, C.SDL_PIXELFORMAT_RGBA32,
		C.SDL_TEXTUREACCESS_STREAMING, C.int(img.Rect.Dx()), C.int(img.Rect.Dy()))
	if texture == nil {
		panic(fmt.Sprintf("SDL_CreateTexture error: %s", C.GoString(C.SDL_GetError())))
	}
	return &sdlImageWrapper{win, texture, offsetX, offsetY, img.Rect.Dx(), img.Rect.Dy(), img}
}

func (i *sdlImageWrapper) Update(rect image.Rectangle) {
	// Pin the Go objects to ensure they are not moved or garbage collected
	texturePointer := i.texture
	windowPointer := i.window
	imagePointer := i.img

	// Ensure that Go objects are pinned before calling the C function
	runtime.KeepAlive(texturePointer)
	runtime.KeepAlive(windowPointer)
	runtime.KeepAlive(imagePointer)

	// Set the background color (clear the screen)
	C.SDL_SetRenderDrawColor(windowPointer.renderer, C.Uint8(0), C.Uint8(0), C.Uint8(0), C.Uint8(255)) // Black color
	C.SDL_RenderClear(windowPointer.renderer)

	// Perform the SDL_UpdateTexture call to upload the image data to the texture
	C.SDL_UpdateTexture(
		texturePointer,                       // texture to update
		nil,                                  // area of the texture to update (nil means entire texture)
		unsafe.Pointer(&imagePointer.Pix[0]), // pixel data from the image
		C.int(i.width*4),                     // pitch (row size in bytes, 4 bytes per pixel for RGBA)
	)

	// Setup source and destination rectangles for rendering
	src := C.SDL_Rect{C.int(rect.Min.X), C.int(rect.Min.Y), C.int(rect.Dx()), C.int(rect.Dy())}
	dst := C.SDL_Rect{C.int(i.offsetX + rect.Min.X), C.int(i.offsetY + rect.Min.Y), C.int(rect.Dx()), C.int(rect.Dy())}

	// Render the texture to the window
	C.SDL_RenderCopy(windowPointer.renderer, texturePointer, &src, &dst)

	// Present the modified renderer on the screen
	C.SDL_RenderPresent(windowPointer.renderer)

	// Ensure that the pointers are kept alive during the C function call
	runtime.KeepAlive(texturePointer)
	runtime.KeepAlive(windowPointer)
	runtime.KeepAlive(imagePointer)
}

func (i *sdlImageWrapper) Delete() {
	C.SDL_DestroyTexture(i.texture)
}

type QuitEvent struct{}
type UnknownEvent struct{}

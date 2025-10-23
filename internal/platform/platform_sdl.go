//go:build !x11

package platform

/*
#cgo pkg-config: sdl2
#include <SDL2/SDL.h>
static inline void my_SDL_DestroyTexture(SDL_Texture* t) {
    SDL_DestroyTexture(t);
}
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

	renderer := createRendererWithProbe(window)
	if renderer == nil {
		C.SDL_DestroyWindow(window)
		C.SDL_Quit()
		panic(fmt.Sprintf("SDL_CreateRenderer error: %s", C.GoString(C.SDL_GetError())))
	}

	return &sdlWindowWrapper{window, renderer, conf.Title, conf.Width, conf.Height}
}

func createRendererWithProbe(window *C.SDL_Window) *C.SDL_Renderer {
	// Najpierw spróbuj akcelerowany bez vsync dla mniejszego opóźnienia
	renderer := C.SDL_CreateRenderer(window, -1, C.SDL_RENDERER_ACCELERATED)
	if renderer != nil && rendererWorks(renderer) {
		return renderer
	}
	if renderer != nil {
		C.SDL_DestroyRenderer(renderer)
	}

	// Druga próba: akcelerowany z vsync
	renderer = C.SDL_CreateRenderer(window, -1, C.SDL_RENDERER_ACCELERATED|C.SDL_RENDERER_PRESENTVSYNC)
	if renderer != nil && rendererWorks(renderer) {
		return renderer
	}
	if renderer != nil {
		C.SDL_DestroyRenderer(renderer)
	}

	// Ostatecznie: software (działał u Ciebie na pewno)
	renderer = C.SDL_CreateRenderer(window, -1, C.SDL_RENDERER_SOFTWARE)
	return renderer
}

func rendererWorks(r *C.SDL_Renderer) bool {
	C.SDL_SetRenderDrawColor(r, 255, 0, 0, 255)
	C.SDL_RenderClear(r)
	C.SDL_RenderPresent(r)
	return true
}

func (w *sdlWindowWrapper) Show() {
	C.SDL_ShowWindow(w.window)
	C.SDL_EventState(C.SDL_QUIT, C.SDL_ENABLE)

	// Pierwsza ramka – żeby kompozytor dostał realną zawartość.
	C.SDL_SetRenderDrawColor(w.renderer, C.Uint8(0), C.Uint8(0), C.Uint8(0), C.Uint8(255))
	C.SDL_RenderClear(w.renderer)
	C.SDL_RenderPresent(w.renderer)
}

func (w *sdlWindowWrapper) Close() {
	C.SDL_DestroyRenderer(w.renderer)
	C.SDL_DestroyWindow(w.window)
	C.SDL_Quit()
	runtime.UnlockOSThread()
}

func (w *sdlWindowWrapper) NextEventTimeout(timeoutMs int) Event {
	var e C.SDL_Event
	if C.SDL_WaitEventTimeout(&e, C.int(timeoutMs)) != 0 {
		return convert(e)
	}
	return TimeoutEvent{} // brak eventu, upłynął timeout
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
	case C.SDL_MOUSEWHEEL:
		wheelEvent := (*C.SDL_MouseWheelEvent)(unsafe.Pointer(&event))
		dx := float64(wheelEvent.x)
		dy := float64(wheelEvent.y)
		if wheelEvent.direction == C.SDL_MOUSEWHEEL_FLIPPED {
			dx = -dx
			dy = -dy
		}
		var mx, my C.int
		C.SDL_GetMouseState(&mx, &my)
		return MouseWheel{
			DeltaX: dx,
			DeltaY: dy,
			X:      int(mx),
			Y:      int(my),
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
		if eventType >= C.SDL_USEREVENT && eventType < C.SDL_LASTEVENT {
			return UnexpectedEvent{}
		}
		// fmt.Printf("Unhandled SDL event type: %d\n", eventType)
		return UnexpectedEvent{}
	}
	return UnexpectedEvent{}
}

// ------------------

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

	C.SDL_SetTextureBlendMode(texture, C.SDL_BLENDMODE_NONE)

	return &sdlImageWrapper{
		window:  win,
		texture: texture,
		offsetX: offsetX,
		offsetY: offsetY,
		width:   img.Rect.Dx(),
		height:  img.Rect.Dy(),
		img:     img,
	}
}

func (i *sdlImageWrapper) Update(rect image.Rectangle) {
	if rect.Empty() {
		return
	}

	if C.SDL_UpdateTexture(
		i.texture,
		nil,
		unsafe.Pointer(&i.img.Pix[0]),
		C.int(i.img.Stride),
	) != 0 {
		fmt.Println("SDL_UpdateTexture error:", C.GoString(C.SDL_GetError()))
	}

	C.SDL_SetRenderDrawColor(i.window.renderer, 0, 0, 0, 255)
	C.SDL_RenderClear(i.window.renderer)

	if C.SDL_RenderCopy(i.window.renderer, i.texture, nil, nil) != 0 {
		fmt.Println("SDL_RenderCopy error:", C.GoString(C.SDL_GetError()))
	}

	C.SDL_RenderPresent(i.window.renderer)

	runtime.KeepAlive(i.texture)
	runtime.KeepAlive(i.window)
	runtime.KeepAlive(i.img)
}

func (i *sdlImageWrapper) Delete() {
	C.my_SDL_DestroyTexture(i.texture)
}

type QuitEvent struct{}
type UnknownEvent struct{}

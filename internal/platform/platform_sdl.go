//go:build (linux || windows || darwin) && !x11 && cgo

package platform

/*
#cgo pkg-config: sdl2
#include <SDL2/SDL.h>
*/
import "C"
import (
	"fmt"
	"runtime"
	"unsafe"
)

type sdlWindowWrapper struct {
	window    *C.SDL_Window
	glContext C.SDL_GLContext
	title     string
	width     int
	height    int
}

func NewPlatformWindowWrapper(conf WindowConfig) PlatformWindowWrapper {
	runtime.LockOSThread()
	if C.SDL_Init(C.SDL_INIT_VIDEO) != 0 {
		panic(fmt.Sprintf("SDL_Init error: %s", C.GoString(C.SDL_GetError())))
	}

	C.SDL_GL_SetAttribute(C.SDL_GL_CONTEXT_MAJOR_VERSION, 3)
	C.SDL_GL_SetAttribute(C.SDL_GL_CONTEXT_MINOR_VERSION, 3)
	C.SDL_GL_SetAttribute(C.SDL_GL_CONTEXT_PROFILE_MASK, C.SDL_GL_CONTEXT_PROFILE_CORE)
	C.SDL_GL_SetAttribute(C.SDL_GL_DOUBLEBUFFER, 1)
	C.SDL_GL_SetAttribute(C.SDL_GL_DEPTH_SIZE, 24)

	cTitle := C.CString(conf.Title)
	defer C.free(unsafe.Pointer(cTitle))

	window := C.SDL_CreateWindow(cTitle, C.SDL_WINDOWPOS_CENTERED, C.SDL_WINDOWPOS_CENTERED,
		C.int(conf.Width), C.int(conf.Height), C.SDL_WINDOW_SHOWN|C.SDL_WINDOW_OPENGL)
	if window == nil {
		panic(fmt.Sprintf("SDL_CreateWindow error: %s", C.GoString(C.SDL_GetError())))
	}

	return &sdlWindowWrapper{
		window: window,
		title:  conf.Title,
		width:  conf.Width,
		height: conf.Height,
	}
}

func (w *sdlWindowWrapper) Show() {
	C.SDL_ShowWindow(w.window)
	C.SDL_EventState(C.SDL_QUIT, C.SDL_ENABLE)
}

func (w *sdlWindowWrapper) Close() {
	if w.glContext != nil {
		C.SDL_GL_DeleteContext(w.glContext)
		w.glContext = nil
	}
	if w.window != nil {
		C.SDL_DestroyWindow(w.window)
		w.window = nil
	}
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

func (w *sdlWindowWrapper) BeginFrame() {
	if w.window == nil {
		return
	}
	if w.glContext == nil {
		ctx := C.SDL_GL_CreateContext(w.window)
		if ctx == nil {
			panic(fmt.Sprintf("SDL_GL_CreateContext error: %s", C.GoString(C.SDL_GetError())))
		}
		w.glContext = ctx
		C.SDL_GL_SetSwapInterval(1)
	}
	if C.SDL_GL_MakeCurrent(w.window, w.glContext) != 0 {
		panic(fmt.Sprintf("SDL_GL_MakeCurrent error: %s", C.GoString(C.SDL_GetError())))
	}
}

func (w *sdlWindowWrapper) EndFrame() {
	if w.window == nil {
		return
	}
	C.SDL_GL_SwapWindow(w.window)
}

func (w *sdlWindowWrapper) GLContext() any {
	return nil
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
		return UnexpectedEvent{}
	}
	return UnexpectedEvent{}
}

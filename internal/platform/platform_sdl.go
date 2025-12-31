//go:build (linux || windows || darwin) && !x11 && cgo

package platform

/*
#cgo pkg-config: sdl2
#include <SDL2/SDL.h>
static inline void my_SDL_DestroyTexture(SDL_Texture* t) {
    SDL_DestroyTexture(t);
}

static inline const char* my_SDL_GetRendererInfo(SDL_Renderer* r, Uint32 *flags) {
    SDL_RendererInfo info;
    if (SDL_GetRendererInfo(r, &info) != 0) {
        return NULL;
    }
    *flags = info.flags;
    return info.name;
}
*/
import "C"
import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"sync"
	"unsafe"
)

type sdlWindowWrapper struct {
	window          *C.SDL_Window
	renderer        *C.SDL_Renderer
	textures        []*sdlTextureImageWrapper
	texturesMu      sync.Mutex
	title           string
	width           int
	height          int
	surfaceFactory  SurfaceFactory
	frameHasUpdates bool
	forcePresent    bool
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

	return &sdlWindowWrapper{
		window:         window,
		renderer:       renderer,
		title:          conf.Title,
		width:          conf.Width,
		height:         conf.Height,
		surfaceFactory: newSDLSurfaceFactory(renderer),
	}
}

func createRendererWithProbe(window *C.SDL_Window) *C.SDL_Renderer {
	// Najbardziej „wypasiona” konfiguracja
	renderer := C.SDL_CreateRenderer(window, -1,
		C.SDL_RENDERER_ACCELERATED|C.SDL_RENDERER_PRESENTVSYNC|C.SDL_RENDERER_TARGETTEXTURE)
	if renderer != nil && rendererWorks(renderer) {
		printRendererInfo(renderer)
		return renderer
	}

	// Spróbuj bez TARGETTEXTURE
	if renderer != nil {
		C.SDL_DestroyRenderer(renderer)
	}
	renderer = C.SDL_CreateRenderer(window, -1,
		C.SDL_RENDERER_ACCELERATED|C.SDL_RENDERER_PRESENTVSYNC)
	if renderer != nil && rendererWorks(renderer) {
		printRendererInfo(renderer)
		return renderer
	}

	// Spróbuj samo ACCELERATED
	if renderer != nil {
		C.SDL_DestroyRenderer(renderer)
	}
	renderer = C.SDL_CreateRenderer(window, -1,
		C.SDL_RENDERER_ACCELERATED)
	if renderer != nil && rendererWorks(renderer) {
		printRendererInfo(renderer)
		return renderer
	}

	// Ostatecznie software
	if renderer != nil {
		C.SDL_DestroyRenderer(renderer)
	}
	renderer = C.SDL_CreateRenderer(window, -1, C.SDL_RENDERER_SOFTWARE)
	if renderer != nil {
		printRendererInfo(renderer)
	}
	return renderer
}

func printRendererInfo(r *C.SDL_Renderer) {
	var flags C.Uint32
	name := C.my_SDL_GetRendererInfo(r, &flags)
	if name == nil {
		fmt.Println("SDL_GetRendererInfo error:", C.GoString(C.SDL_GetError()))
		return
	}

	fmt.Printf("SDL renderer backend: %s\n", C.GoString(name))
	if flags&C.SDL_RENDERER_ACCELERATED != 0 {
		fmt.Println(" - accelerated (GPU)")
	}
	if flags&C.SDL_RENDERER_SOFTWARE != 0 {
		fmt.Println(" - software (CPU)")
	}
	if flags&C.SDL_RENDERER_PRESENTVSYNC != 0 {
		fmt.Println(" - vsync enabled")
	}
	if flags&C.SDL_RENDERER_TARGETTEXTURE != 0 {
		fmt.Println(" - target texture supported")
	}
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

	if w.renderer != nil {
		// Pierwsza ramka – żeby kompozytor dostał realną zawartość.
		C.SDL_SetRenderDrawColor(w.renderer, C.Uint8(0), C.Uint8(0), C.Uint8(0), C.Uint8(255))
		C.SDL_RenderClear(w.renderer)
		C.SDL_RenderPresent(w.renderer)
	}
}

func (w *sdlWindowWrapper) Close() {
	if w.renderer != nil {
		C.SDL_DestroyRenderer(w.renderer)
	}
	C.SDL_DestroyWindow(w.window)
	C.SDL_Quit()
	runtime.UnlockOSThread()
}

func (w *sdlWindowWrapper) NextEventTimeout(timeoutMs int) Event {
	var e C.SDL_Event
	if C.SDL_WaitEventTimeout(&e, C.int(timeoutMs)) != 0 {
		if (*(*C.Uint32)(unsafe.Pointer(&e))) == C.SDL_WINDOWEVENT {
			windowEvent := (*C.SDL_WindowEvent)(unsafe.Pointer(&e))
			if windowEvent.event == C.SDL_WINDOWEVENT_EXPOSED {
				w.forcePresent = true
			}
		}
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
	return newSDLTextureImageWrapper(w, img, offsetX, offsetY)
}

func (w *sdlWindowWrapper) SurfaceFactory() SurfaceFactory {
	return w.surfaceFactory
}

func (w *sdlWindowWrapper) BeginFrame() {
	if w.renderer == nil {
		return
	}
	w.frameHasUpdates = false
}

func (w *sdlWindowWrapper) EndFrame() {
	if w.renderer == nil {
		return
	}
	if !w.frameHasUpdates && !w.forcePresent {
		return
	}

	C.SDL_SetRenderDrawColor(w.renderer, 0, 0, 0, 255)
	C.SDL_RenderClear(w.renderer)

	for _, tex := range w.textureSnapshot() {
		if tex == nil || tex.texture == nil || tex.img == nil {
			continue
		}
		dstRect := C.SDL_Rect{
			x: C.int(tex.offsetX),
			y: C.int(tex.offsetY),
			w: C.int(tex.img.Rect.Dx()),
			h: C.int(tex.img.Rect.Dy()),
		}
		if C.SDL_RenderCopy(w.renderer, tex.texture, nil, &dstRect) != 0 {
			fmt.Println("SDL_RenderCopy error:", C.GoString(C.SDL_GetError()))
		}
	}

	C.SDL_RenderPresent(w.renderer)
	w.forcePresent = false
}

type sdlSurfaceFactory struct {
	renderer *C.SDL_Renderer
}

func newSDLSurfaceFactory(renderer *C.SDL_Renderer) SurfaceFactory {
	return &sdlSurfaceFactory{renderer: renderer}
}

func (f *sdlSurfaceFactory) New(width, height int) Surface {
	return &sdlTextureSurface{
		renderer: f.renderer,
		img:      image.NewRGBA(image.Rect(0, 0, width, height)),
	}
}

type sdlTextureSurface struct {
	renderer *C.SDL_Renderer
	img      *image.RGBA
}

func (s *sdlTextureSurface) ColorModel() color.Model     { return s.img.ColorModel() }
func (s *sdlTextureSurface) Bounds() image.Rectangle     { return s.img.Bounds() }
func (s *sdlTextureSurface) At(x, y int) color.Color     { return s.img.At(x, y) }
func (s *sdlTextureSurface) Set(x, y int, c color.Color) { s.img.Set(x, y, c) }
func (s *sdlTextureSurface) RGBA() *image.RGBA           { return s.img }

type sdlTextureImageWrapper struct {
	window  *sdlWindowWrapper
	texture *C.SDL_Texture
	img     *image.RGBA
	offsetX int
	offsetY int
}

func newSDLTextureImageWrapper(win *sdlWindowWrapper, img *image.RGBA, offsetX, offsetY int) *sdlTextureImageWrapper {
	wrapper := &sdlTextureImageWrapper{
		window:  win,
		img:     img,
		offsetX: offsetX,
		offsetY: offsetY,
	}

	if win == nil || win.renderer == nil || img == nil {
		return wrapper
	}

	width := img.Rect.Dx()
	height := img.Rect.Dy()
	if width <= 0 || height <= 0 {
		return wrapper
	}

	texture := C.SDL_CreateTexture(win.renderer, C.SDL_PIXELFORMAT_RGBA32, C.SDL_TEXTUREACCESS_STREAMING, C.int(width), C.int(height))
	if texture == nil {
		fmt.Println("SDL_CreateTexture error:", C.GoString(C.SDL_GetError()))
		return wrapper
	}

	if C.SDL_SetTextureBlendMode(texture, C.SDL_BLENDMODE_BLEND) != 0 {
		fmt.Println("SDL_SetTextureBlendMode error:", C.GoString(C.SDL_GetError()))
	}

	wrapper.texture = texture
	win.registerTexture(wrapper)
	return wrapper
}

func (i *sdlTextureImageWrapper) Update(rect image.Rectangle) {
	if i == nil || i.texture == nil || i.img == nil {
		return
	}

	if rect.Empty() {
		return
	}
	rect = rect.Intersect(i.img.Rect)
	if rect.Empty() {
		return
	}

	relX := rect.Min.X - i.img.Rect.Min.X
	relY := rect.Min.Y - i.img.Rect.Min.Y
	offset := relY*i.img.Stride + relX*4
	if offset < 0 || offset >= len(i.img.Pix) {
		return
	}

	pixels := unsafe.Pointer(&i.img.Pix[offset])
	sdlRect := C.SDL_Rect{
		x: C.int(relX),
		y: C.int(relY),
		w: C.int(rect.Dx()),
		h: C.int(rect.Dy()),
	}
	if C.SDL_UpdateTexture(i.texture, &sdlRect, pixels, C.int(i.img.Stride)) != 0 {
		fmt.Println("SDL_UpdateTexture error:", C.GoString(C.SDL_GetError()))
		return
	}

	if i.window != nil {
		i.window.frameHasUpdates = true
	}

	runtime.KeepAlive(i.window)
	runtime.KeepAlive(i.img)
}

func (i *sdlTextureImageWrapper) Delete() {
	if i != nil && i.window != nil {
		i.window.unregisterTexture(i)
	}
	if i != nil && i.texture != nil {
		C.my_SDL_DestroyTexture(i.texture)
		i.texture = nil
	}
	if i != nil {
		i.img = nil
		i.window = nil
	}
}

type QuitEvent struct{}
type UnknownEvent struct{}

func (w *sdlWindowWrapper) registerTexture(tex *sdlTextureImageWrapper) {
	if w == nil || tex == nil {
		return
	}
	w.texturesMu.Lock()
	w.textures = append(w.textures, tex)
	w.texturesMu.Unlock()
}

func (w *sdlWindowWrapper) unregisterTexture(tex *sdlTextureImageWrapper) {
	if w == nil || tex == nil {
		return
	}
	w.texturesMu.Lock()
	for idx, current := range w.textures {
		if current == tex {
			copy(w.textures[idx:], w.textures[idx+1:])
			w.textures[len(w.textures)-1] = nil
			w.textures = w.textures[:len(w.textures)-1]
			break
		}
	}
	w.texturesMu.Unlock()
	w.forcePresent = true
}

func (w *sdlWindowWrapper) textureSnapshot() []*sdlTextureImageWrapper {
	if w == nil {
		return nil
	}
	w.texturesMu.Lock()
	if len(w.textures) == 0 {
		w.texturesMu.Unlock()
		return nil
	}
	out := make([]*sdlTextureImageWrapper, len(w.textures))
	copy(out, w.textures)
	w.texturesMu.Unlock()
	return out
}

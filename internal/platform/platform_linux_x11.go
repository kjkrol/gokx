//go:build linux && x11

package platform

/*
#cgo LDFLAGS: -lX11 -lEGL
#include <stdlib.h>
#include <X11/Xlib.h>
#include <EGL/egl.h>
#include <EGL/eglext.h>

void destroyImage(XImage *image) {
    if (image) {
        // if (image->data) {
        //     free(image->data);
        // }
        free(image);
    }
}
static int getConnectionNumber(Display* d) {
    return ConnectionNumber(d);
}

*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"image"
	"syscall"
	"time"
	"unsafe"
)

// ----------------------------------------------------------------------------

const (
	NoEventMask              = 0
	KeyPressMask             = 1 << 0 // Listen for key press events.
	KeyReleaseMask           = 1 << 1 // Listen for mouse button press events.
	ButtonPressMask          = 1 << 2 // Listen for mouse button press events.
	ButtonReleaseMask        = 1 << 3 // Listen for mouse button release events.
	EnterWindowMask          = 1 << 4 // Listen for the pointer entering the window.
	LeaveWindowMask          = 1 << 5 // Listen for the pointer leaving the window.
	PointerMotionMask        = 1 << 6 // Listen for mouse movement events
	PointerMotionHintMask    = 1 << 7
	Button1MotionMask        = 1 << 8
	Button2MotionMask        = 1 << 9
	Button3MotionMask        = 1 << 10
	Button4MotionMask        = 1 << 11
	Button5MotionMask        = 1 << 12
	ButtonMotionMask         = 1 << 13
	KeymapStateMask          = 1 << 14
	ExposureMask             = 1 << 15 // Listen for expose events (redraw window contents).
	VisibilityChangeMask     = 1 << 16
	StructureNotifyMask      = 1 << 17 // Listen for structural changes like resizing or closing the window.
	ResizeRedirectMask       = 1 << 18
	SubstructureNotifyMask   = 1 << 19
	SubstructureRedirectMask = 1 << 20
	FocusChangeMask          = 1 << 21
	PropertyChangeMask       = 1 << 22
	ColormapChangeMask       = 1 << 23
	OwnerGrabButtonMask      = 1 << 24

	DefaultMask = KeyPressMask | KeyReleaseMask | ButtonPressMask | ButtonReleaseMask |
		PointerMotionMask | ExposureMask | EnterWindowMask |
		LeaveWindowMask | StructureNotifyMask | 33
)

// ----------------------------------------------------------------------------

func NewPlatformWindowWrapper(conf WindowConfig) PlatformWindowWrapper {

	conn, err := newXConnection()
	if err != nil {
		fmt.Println(err)
		return nil
	}

	window := C.XCreateSimpleWindow(
		conn.display,
		conn.rootWindow,
		C.int(conf.PositionX),
		C.int(conf.PositionY),
		C.uint(conf.Width),
		C.uint(conf.Height),
		C.uint(conf.BorderWidth),
		C.XBlackPixel(conn.display, conn.screen),
		C.XWhitePixel(conn.display, conn.screen),
	)

	title := C.CString(conf.Title)
	C.XStoreName(conn.display, window, title)

	return &x11WindowWrapper{
		conn:           conn,
		window:         window,
		title:          title,
		surfaceFactory: DefaultSurfaceFactory(),
	}
}

// ----------------------------------------------------------------------------

type xConnection struct {
	display    *C.Display
	screen     C.int
	rootWindow C.Window
	gc         C.GC
}

func newXConnection() (*xConnection, error) {
	display := C.XOpenDisplay(nil)
	if display == nil {
		return nil, fmt.Errorf("unable to open X display")
	}

	screen := C.XDefaultScreen(display)
	rootWindow := C.XRootWindow(display, screen)
	return &xConnection{
		display:    display,
		screen:     screen,
		rootWindow: rootWindow,
		gc:         C.XDefaultGC(display, screen),
	}, nil
}

func (c *xConnection) Close() {
	C.XDestroyWindow(c.display, c.rootWindow)
	C.XCloseDisplay(c.display)
}

// ----------------------------------------------------------------------------

type x11ImageWrapper struct {
	win              *x11WindowWrapper
	xImage           *C.XImage
	offsetX, offsetY int

	src   *image.RGBA    // źródło (to Twoje img)
	buf   unsafe.Pointer // C.malloc() – dane dla XImage
	pitch int            // bajtów na wiersz (Stride)
	w, h  int            // rozmiar całego obrazu
}

func newx11ImageWrapper(win *x11WindowWrapper, img *image.RGBA, offsetX, offsetY int) *x11ImageWrapper {
	w := img.Rect.Dx()
	h := img.Rect.Dy()
	pitch := img.Stride
	size := C.size_t(h * pitch)

	data := C.malloc(size)

	xImage := C.XCreateImage(
		win.conn.display,
		C.XDefaultVisual(win.conn.display, win.conn.screen),
		24, // depth
		C.ZPixmap,
		0,               // offset
		(*C.char)(data), // -> C-bufor
		C.uint(w), C.uint(h),
		32,           // bitmap_pad
		C.int(pitch), // bytes_per_line
	)

	return &x11ImageWrapper{
		win: win, xImage: xImage,
		offsetX: offsetX, offsetY: offsetY,
		src: img, buf: data, pitch: pitch, w: w, h: h,
	}
}

func (xw *x11ImageWrapper) Update(rect image.Rectangle) {
	r := rect.Intersect(image.Rect(0, 0, xw.w, xw.h))
	if r.Empty() {
		return
	}

	bufSize := xw.h * xw.pitch
	dst := (*[1 << 30]byte)(xw.buf)[:bufSize:bufSize]

	copyRectRGBAtoBGRA(dst, xw.pitch, xw.src, r)

	C.XPutImage(
		xw.win.conn.display,
		xw.win.window,
		xw.win.conn.gc,
		xw.xImage,
		C.int(r.Min.X), C.int(r.Min.Y), // src_x, src_y
		C.int(xw.offsetX+r.Min.X), C.int(xw.offsetY+r.Min.Y), // dst_x, dst_y
		C.uint(r.Dx()), C.uint(r.Dy()),
	)
}

func copyRectRGBAtoBGRA(dst []byte, dstStride int, src *image.RGBA, rect image.Rectangle) {
	sx0 := rect.Min.X - src.Rect.Min.X
	sy0 := rect.Min.Y - src.Rect.Min.Y
	w := rect.Dx()
	h := rect.Dy()
	if w <= 0 || h <= 0 {
		return
	}

	rowBytes := w * 4
	srcStride := src.Stride
	baseDstOffset := rect.Min.Y*dstStride + rect.Min.X*4

	for row := 0; row < h; row++ {
		sOff := (sy0+row)*srcStride + sx0*4
		dOff := baseDstOffset + row*dstStride

		s := src.Pix[sOff : sOff+rowBytes]
		d := dst[dOff : dOff+rowBytes]

		for si, di := 0, 0; si < rowBytes; si, di = si+4, di+4 {
			pix := binary.LittleEndian.Uint32(s[si:])
			swapped := (pix & 0xFF00FF00) | ((pix & 0x000000FF) << 16) | ((pix & 0x00FF0000) >> 16)
			binary.LittleEndian.PutUint32(d[di:], swapped)
		}
	}
}

func (xw *x11ImageWrapper) Delete() {
	if xw.xImage != nil {
		if xw.xImage.data != nil {
			C.free(unsafe.Pointer(xw.xImage.data))
			xw.xImage.data = nil
		}
		C.destroyImage(xw.xImage)
		xw.xImage = nil
	}
	xw.buf = nil
	xw.src = nil
}

// ----------------------------------------------------------------------------

type x11WindowWrapper struct {
	conn           *xConnection
	window         C.Window
	title          *C.char
	fd             int
	readFD         syscall.FdSet
	timeval        syscall.Timeval
	surfaceFactory SurfaceFactory
	eglDisplay     C.EGLDisplay
	eglConfig      C.EGLConfig
	eglSurface     C.EGLSurface
	eglContext     C.EGLContext
}

func (w *x11WindowWrapper) Show() {
	C.XMapWindow(w.conn.display, w.window)

	wmDeleteWindow := C.XInternAtom(w.conn.display, C.CString("WM_DELETE_WINDOW"), 0)
	C.XSetWMProtocols(w.conn.display, w.window, &wmDeleteWindow, 1)
	C.XSelectInput(w.conn.display, w.window, DefaultMask)
}
func (w *x11WindowWrapper) Close() {
	w.destroyEGL()
	C.XDestroyWindow(w.conn.display, w.window)
	C.free(unsafe.Pointer(w.title))
	w.conn.Close()
	w.conn = nil
}

func (w *x11WindowWrapper) NextEventTimeout(timeoutMs int) Event {
	if w.fd == 0 {
		w.fd = int(C.getConnectionNumber(w.conn.display))
	}

	// Drain queued events without waiting on select.
	if C.XPending(w.conn.display) > 0 {
		var ev C.XEvent
		C.XNextEvent(w.conn.display, &ev)
		return convert(ev)
	}

	if timeoutMs < 0 {
		timeoutMs = 0
	}

	duration := time.Duration(timeoutMs) * time.Millisecond
	w.timeval = syscall.NsecToTimeval(duration.Nanoseconds())

	for i := range w.readFD.Bits {
		w.readFD.Bits[i] = 0
	}
	FD_SET(w.fd, &w.readFD)

	n, err := syscall.Select(w.fd+1, &w.readFD, nil, nil, &w.timeval)
	if err != nil || n == 0 {
		return TimeoutEvent{}
	}

	var ev C.XEvent
	if C.XPending(w.conn.display) > 0 {
		C.XNextEvent(w.conn.display, &ev)
		return convert(ev)
	}
	return TimeoutEvent{}
}

func (w *x11WindowWrapper) SurfaceFactory() SurfaceFactory {
	return w.surfaceFactory
}

func (w *x11WindowWrapper) BeginFrame() {
	if w.conn == nil {
		return
	}
	if w.eglDisplay == eglNoDisplay() {
		w.initEGL()
		return
	}
	if w.eglSurface != eglNoSurface() && w.eglContext != eglNoContext() {
		if C.eglMakeCurrent(w.eglDisplay, w.eglSurface, w.eglSurface, w.eglContext) == C.EGL_FALSE {
			panic(fmt.Sprintf("EGL: eglMakeCurrent failed: %v", eglError()))
		}
	}
}

func (w *x11WindowWrapper) EndFrame() {
	if w.eglDisplay == eglNoDisplay() || w.eglSurface == eglNoSurface() {
		return
	}
	if C.eglSwapBuffers(w.eglDisplay, w.eglSurface) == C.EGL_FALSE {
		panic(fmt.Sprintf("EGL: eglSwapBuffers failed: %v", eglError()))
	}
}
func (w *x11WindowWrapper) GLContext() any {
	return nil
}

func (w *x11WindowWrapper) NewPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) PlatformImageWrapper {
	return newx11ImageWrapper(w, img, offsetX, offsetY)
}

// ----------------------------------------------------------------------------

func (w *x11WindowWrapper) initEGL() {
	if w.conn == nil || w.conn.display == nil {
		return
	}
	if w.eglDisplay != eglNoDisplay() {
		return
	}

	display := C.eglGetDisplay(C.EGLNativeDisplayType(unsafe.Pointer(w.conn.display)))
	if display == eglNoDisplay() {
		panic(fmt.Sprintf("EGL: eglGetDisplay failed: %v", eglError()))
	}

	if C.eglInitialize(display, nil, nil) == C.EGL_FALSE {
		panic(fmt.Sprintf("EGL: eglInitialize failed: %v", eglError()))
	}

	if C.eglBindAPI(C.EGL_OPENGL_API) == C.EGL_FALSE {
		panic(fmt.Sprintf("EGL: eglBindAPI failed: %v", eglError()))
	}

	attrs := []C.EGLint{
		C.EGL_SURFACE_TYPE, C.EGL_WINDOW_BIT,
		C.EGL_RENDERABLE_TYPE, C.EGL_OPENGL_BIT,
		C.EGL_RED_SIZE, 8,
		C.EGL_GREEN_SIZE, 8,
		C.EGL_BLUE_SIZE, 8,
		C.EGL_ALPHA_SIZE, 8,
		C.EGL_DEPTH_SIZE, 24,
		C.EGL_STENCIL_SIZE, 8,
		C.EGL_NONE,
	}
	var config C.EGLConfig
	var num C.EGLint
	if C.eglChooseConfig(display, &attrs[0], &config, 1, &num) == C.EGL_FALSE || num == 0 {
		panic(fmt.Sprintf("EGL: eglChooseConfig failed: %v", eglError()))
	}

	surface := C.eglCreateWindowSurface(display, config, C.EGLNativeWindowType(w.window), nil)
	if surface == eglNoSurface() {
		panic(fmt.Sprintf("EGL: eglCreateWindowSurface failed: %v", eglError()))
	}

	ctxAttrs := []C.EGLint{
		C.EGL_CONTEXT_MAJOR_VERSION, 3,
		C.EGL_CONTEXT_MINOR_VERSION, 3,
		C.EGL_NONE,
	}
	context := C.eglCreateContext(display, config, eglNoContext(), &ctxAttrs[0])
	if context == eglNoContext() {
		panic(fmt.Sprintf("EGL: eglCreateContext failed: %v", eglError()))
	}

	if C.eglMakeCurrent(display, surface, surface, context) == C.EGL_FALSE {
		panic(fmt.Sprintf("EGL: eglMakeCurrent failed: %v", eglError()))
	}
	C.eglSwapInterval(display, 1)

	w.eglDisplay = display
	w.eglConfig = config
	w.eglSurface = surface
	w.eglContext = context
}

func (w *x11WindowWrapper) destroyEGL() {
	if w.eglDisplay == eglNoDisplay() {
		return
	}
	C.eglMakeCurrent(w.eglDisplay, eglNoSurface(), eglNoSurface(), eglNoContext())
	if w.eglContext != eglNoContext() {
		C.eglDestroyContext(w.eglDisplay, w.eglContext)
		w.eglContext = eglNoContext()
	}
	if w.eglSurface != eglNoSurface() {
		C.eglDestroySurface(w.eglDisplay, w.eglSurface)
		w.eglSurface = eglNoSurface()
	}
	C.eglTerminate(w.eglDisplay)
	w.eglDisplay = eglNoDisplay()
}

func eglError() error {
	return fmt.Errorf("0x%04x", uint32(C.eglGetError()))
}

func eglNoDisplay() C.EGLDisplay {
	return C.EGLDisplay(unsafe.Pointer(nil))
}

func eglNoSurface() C.EGLSurface {
	return C.EGLSurface(unsafe.Pointer(nil))
}

func eglNoContext() C.EGLContext {
	return C.EGLContext(unsafe.Pointer(nil))
}

func decodeKeyEvent(keyEvent *C.XKeyEvent) (uint64, string) {
	keysym := C.XLookupKeysym(keyEvent, 0)
	char := C.XKeysymToString(keysym)
	label := C.GoString(char)
	return uint64(keysym), label
}

func convert(event C.XEvent) Event {
	switch eventType := (*C.XAnyEvent)(unsafe.Pointer(&event))._type; eventType {
	case 2:
		event := (*C.XKeyEvent)(unsafe.Pointer(&event))
		code, label := decodeKeyEvent(event)
		return KeyPress{Code: code, Label: label}
	case 3:
		event := (*C.XKeyEvent)(unsafe.Pointer(&event))
		code, label := decodeKeyEvent(event)
		return KeyRelease{Code: code, Label: label}
	case 4:
		event := (*C.XButtonEvent)(unsafe.Pointer(&event))
		if dx, dy, ok := x11WheelDelta(uint(event.button)); ok {
			return MouseWheel{DeltaX: dx, DeltaY: dy, X: int(event.x), Y: int(event.y)}
		}
		return ButtonPress{Button: uint32(event.button), X: int(event.x), Y: int(event.y)}
	case 5:
		event := (*C.XButtonEvent)(unsafe.Pointer(&event))
		if _, _, ok := x11WheelDelta(uint(event.button)); ok {
			return UnexpectedEvent{}
		}
		return ButtonRelease{Button: uint32(event.button), X: int(event.x), Y: int(event.y)}
	case 6:
		event := (*C.XButtonEvent)(unsafe.Pointer(&event))
		return MotionNotify{X: int(event.x), Y: int(event.y)}
	case 7:
		return EnterNotify{}
	case 8:
		return LeaveNotify{}
	case 12:
		return Expose{}
	case 16:
		return CreateNotify{}
	case 17:
		return DestroyNotify{}
	case 33:
		return ClientMessage{}
	default:
		// fmt.Printf("Unhandled event type: %d\n", eventType)
		return UnexpectedEvent{}
	}
}

func FD_SET(fd int, p *syscall.FdSet) {
	p.Bits[fd/64] |= 1 << (uint(fd) % 64)
}

func x11WheelDelta(button uint) (float64, float64, bool) {
	switch button {
	case 4:
		return 0, 1, true
	case 5:
		return 0, -1, true
	case 6:
		return -1, 0, true
	case 7:
		return 1, 0, true
	default:
		return 0, 0, false
	}
}

//go:build linux && x11

package platform

/*
#cgo LDFLAGS: -lX11
#include <stdlib.h>
#include <X11/Xlib.h>

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
		conn:   conn,
		window: window,
		title:  title,
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

	// C-bufor na piksele dla XImage
	data := C.malloc(size)

	// XImage wskazuje na C-bufor (NIE na Go slice!)
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
	// przycinamy rect do granic obrazu
	r := rect.Intersect(image.Rect(0, 0, xw.w, xw.h))
	if r.Empty() {
		return
	}

	// zmapuj C-bufor na []byte
	bufSize := xw.h * xw.pitch
	dst := (*[1 << 30]byte)(xw.buf)[:bufSize:bufSize]

	copyRectRGBAtoBGRA(dst, xw.pitch, xw.src, r)

	// wyślij zmieniony wycinek
	C.XPutImage(
		xw.win.conn.display,
		xw.win.window,
		xw.win.conn.gc,
		xw.xImage,
		C.int(r.Min.X), C.int(r.Min.Y), // src_x, src_y
		C.int(xw.offsetX+r.Min.X), C.int(xw.offsetY+r.Min.Y), // dst_x, dst_y
		C.uint(r.Dx()), C.uint(r.Dy()),
	)
	// C.XFlush(xw.win.conn.display)
}

// kopiuje wycinek rect ze źródła RGBA do dst (BGRA) z uwzględnieniem stride’ów
func copyRectRGBAtoBGRA(dst []byte, dstStride int, src *image.RGBA, rect image.Rectangle) {
	// przesunięcia względem Rect źródła
	sx0 := rect.Min.X - src.Rect.Min.X
	sy0 := rect.Min.Y - src.Rect.Min.Y
	w := rect.Dx()
	h := rect.Dy()

	for row := 0; row < h; row++ {
		sOff := (sy0+row)*src.Stride + sx0*4
		dOff := (rect.Min.Y+row)*dstStride + rect.Min.X*4

		s := src.Pix[sOff:]
		d := dst[dOff:]

		// po pikselu: RGBA(s) -> BGRA(d)
		for x := 0; x < w; x++ {
			sr := s[4*x+0]
			sg := s[4*x+1]
			sb := s[4*x+2]
			sa := s[4*x+3]

			d[4*x+0] = sb // B
			d[4*x+1] = sg // G
			d[4*x+2] = sr // R
			d[4*x+3] = sa // A (przy depth=24 alpha zwykle ignorowana, ale zostawiamy)
		}
	}
}

func (xw *x11ImageWrapper) Delete() {
	if xw.xImage != nil {
		if xw.xImage.data != nil {
			C.free(unsafe.Pointer(xw.xImage.data)) // najpierw dane
			xw.xImage.data = nil
		}
		C.destroyImage(xw.xImage) // potem struktura XImage
		xw.xImage = nil
	}
	xw.buf = nil
	xw.src = nil
}

// ----------------------------------------------------------------------------

type x11WindowWrapper struct {
	conn   *xConnection
	window C.Window
	title  *C.char
}

func (w *x11WindowWrapper) Show() {
	C.XMapWindow(w.conn.display, w.window)

	wmDeleteWindow := C.XInternAtom(w.conn.display, C.CString("WM_DELETE_WINDOW"), 0)
	C.XSetWMProtocols(w.conn.display, w.window, &wmDeleteWindow, 1)
	C.XSelectInput(w.conn.display, w.window, DefaultMask)
}
func (w *x11WindowWrapper) Close() {
	C.XDestroyWindow(w.conn.display, w.window)
	C.free(unsafe.Pointer(w.title))
	w.conn.Close()
	w.conn = nil
}

func (w *x11WindowWrapper) NextEventTimeout(timeoutMs int) Event {
	fd := int(C.getConnectionNumber(w.conn.display))

	// przygotuj timeout
	tv := syscall.NsecToTimeval((time.Duration(timeoutMs) * time.Millisecond).Nanoseconds())

	var readfds syscall.FdSet
	FD_SET(fd, &readfds)

	n, err := syscall.Select(fd+1, &readfds, nil, nil, &tv)
	if err != nil || n == 0 {
		return TimeoutEvent{} // brak eventu
	}

	var ev C.XEvent
	if C.XPending(w.conn.display) > 0 {
		C.XNextEvent(w.conn.display, &ev)
		return convert(ev)
	}
	return TimeoutEvent{}
}

func (w *x11WindowWrapper) NextEvent() Event {
	return w.NextEventTimeout(16) // ~60fps, jak SDL
}

func (w *x11WindowWrapper) NewPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) PlatformImageWrapper {
	return newx11ImageWrapper(w, img, offsetX, offsetY)
}

// ----------------------------------------------------------------------------

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
		return ButtonPress{Button: uint32(event.button), X: int(event.x), Y: int(event.y)}
	case 5:
		event := (*C.XButtonEvent)(unsafe.Pointer(&event))
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

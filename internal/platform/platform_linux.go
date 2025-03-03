//go:build linux

package platform

/*
#cgo CFLAGS: -I/usr/include/X11
#cgo LDFLAGS: -lX11
#include <stdlib.h>
#include <X11/Xlib.h>
#include <X11/X.h>

void destroyImage(XImage *image) {
    if (image) {
        // if (image->data) {
        //     free(image->data);
        // }
        free(image);
    }
}
*/
import "C"

import (
	"fmt"
	"image"
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

	return &linuxWindowWrapper{
		conn:   conn,
		window: window,
		title:  title,
	}
}

func newLinuxImageWrapper(win *linuxWindowWrapper, img *image.RGBA, offsetX, offsetY int) *linuxImageWrapper {
	xImage := C.XCreateImage(
		win.conn.display,
		C.XDefaultVisual(win.conn.display, win.conn.screen),
		24,
		C.ZPixmap,
		0,
		(*C.char)(unsafe.Pointer(&img.Pix[0])),
		C.uint(img.Rect.Dx()),
		C.uint(img.Rect.Dy()),
		32,
		C.int(img.Stride),
	)
	return &linuxImageWrapper{win, xImage, offsetX, offsetY}
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

type linuxImageWrapper struct {
	win              *linuxWindowWrapper
	xImage           *C.XImage
	offsetX, offsetY int
}

func (xw *linuxImageWrapper) Update(rect image.Rectangle) {
	C.XPutImage(
		xw.win.conn.display,
		xw.win.window,
		xw.win.conn.gc,
		xw.xImage,
		C.int(rect.Min.X), C.int(rect.Min.Y),
		C.int(xw.offsetX+rect.Min.X),
		C.int(xw.offsetY+rect.Min.Y),
		C.uint(rect.Dx()),
		C.uint(rect.Dy()),
	)
	// C.XFlush(xw.win.conn.display)
}

func (xw *linuxImageWrapper) Delete() {
	C.destroyImage(xw.xImage)
	xw.xImage = nil
}

// ----------------------------------------------------------------------------

type linuxWindowWrapper struct {
	conn   *xConnection
	window C.Window
	title  *C.char
}

func (w *linuxWindowWrapper) Show() {
	C.XMapWindow(w.conn.display, w.window)

	wmDeleteWindow := C.XInternAtom(w.conn.display, C.CString("WM_DELETE_WINDOW"), 0)
	C.XSetWMProtocols(w.conn.display, w.window, &wmDeleteWindow, 1)
	C.XSelectInput(w.conn.display, w.window, DefaultMask)
}
func (w *linuxWindowWrapper) Close() {
	C.XDestroyWindow(w.conn.display, w.window)
	C.free(unsafe.Pointer(w.title))
	w.conn.Close()
	w.conn = nil
}
func (w *linuxWindowWrapper) NextEvent() Event {
	var x11Event C.XEvent
	C.XNextEvent(w.conn.display, &x11Event)
	return convert(x11Event)
}

func (w *linuxWindowWrapper) NewPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) PlatformImageWrapper {
	return newLinuxImageWrapper(w, img, offsetX, offsetY)
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

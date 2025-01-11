//go:build darwin

package xgraph

import (
	"fmt"
	"image"
)

// ----------------------------------------------------------------------------

type osxWindowWrapper struct{}

func newPlatformWindowWrapper(conf WindowConfig) platformWindowWrapper {
	fmt.Println("OSX window wrapper", conf)
	return &osxWindowWrapper{}
}

func (w *osxWindowWrapper) show()            {}
func (w *osxWindowWrapper) close()           {}
func (w *osxWindowWrapper) nextEvent() Event { return nil }
func (w *osxWindowWrapper) newPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) platformImageWrapper {
	return nil
}

// ----------------------------------------------------------------------------

type osxImageWrapper struct {
	win              *osxImageWrapper
	offsetX, offsetY int
}

func (xw *osxImageWrapper) update(rect image.Rectangle) {}

func (xw *osxImageWrapper) delete() {}

// ----------------------------------------------------------------------------

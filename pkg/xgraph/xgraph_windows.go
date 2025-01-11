//go:build windows

package xgraph

import (
	"fmt"
	"image"
)

// ----------------------------------------------------------------------------

type windowsWindowWrapper struct{}

func newPlatformWindowWrapper(conf WindowConfig) platformWindowWrapper {
	fmt.Println("Windows window wrapper", conf)
	return &windowsWindowWrapper{}
}

func (w *windowsWindowWrapper) show()            {}
func (w *windowsWindowWrapper) close()           {}
func (w *windowsWindowWrapper) nextEvent() Event { return nil }
func (w *windowsWindowWrapper) newPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) platformImageWrapper {
	return nil
}

// ----------------------------------------------------------------------------

type windowsImageWrapper struct {
	win              *windowsImageWrapper
	offsetX, offsetY int
}

func (xw *windowsImageWrapper) update(rect image.Rectangle) {}

func (xw *windowsImageWrapper) delete() {}

// ----------------------------------------------------------------------------

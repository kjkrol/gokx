//go:build windows

package platform

import (
	"fmt"
	"image"
)

// ----------------------------------------------------------------------------

type windowsWindowWrapper struct{}

func NewPlatformWindowWrapper(conf WindowConfig) PlatformWindowWrapper {
	fmt.Println("Windows window wrapper", conf)
	return &windowsWindowWrapper{}
}

func (w *windowsWindowWrapper) Show()            {}
func (w *windowsWindowWrapper) Close()           {}
func (w *windowsWindowWrapper) NextEvent() Event { return nil }
func (w *windowsWindowWrapper) NewPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) PlatformImageWrapper {
	return nil
}

// ----------------------------------------------------------------------------

type windowsImageWrapper struct {
	win              *windowsImageWrapper
	offsetX, offsetY int
}

func (xw *windowsImageWrapper) Update(rect image.Rectangle) {}

func (xw *windowsImageWrapper) Delete() {}

// ----------------------------------------------------------------------------

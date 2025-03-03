package platform

import "image"

type WindowConfig struct {
	PositionX   int
	PositionY   int
	Width       int
	Height      int
	BorderWidth int
	Title       string
}

type PlatformWindowWrapper interface {
	Show()
	Close()
	NextEvent() Event
	NewPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) PlatformImageWrapper
}

type PlatformImageWrapper interface {
	Update(rect image.Rectangle)
	Delete()
}

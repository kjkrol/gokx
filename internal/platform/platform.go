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
	NextEventTimeout(timeoutMs int) Event
	BeginFrame()
	EndFrame()
	GLContext() any
}

type PlatformImageWrapper interface {
	Update(rect image.Rectangle)
	Delete()
}

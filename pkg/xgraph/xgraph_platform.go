package xgraph

import "image"

type platformWindowWrapper interface {
	show()
	close()
	nextEvent() Event
	newPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) platformImageWrapper
}

type platformImageWrapper interface {
	update(rect image.Rectangle)
	delete()
}

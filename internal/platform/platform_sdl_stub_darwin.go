//go:build darwin && !x11 && !cgo

package platform

func NewPlatformWindowWrapper(conf WindowConfig) PlatformWindowWrapper {
	panic("platform: SDL backend requires cgo; rebuild with CGO_ENABLED=1")
}

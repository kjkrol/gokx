package platform

import (
	"image"
	"image/color"
)

// Surface represents a drawable target that can provide access to its underlying RGBA buffer.
// Initially it wraps the standard library's image.RGBA, but the interface is kept narrow so
// alternative backends can plug in later.
type Surface interface {
	ColorModel() color.Model
	Bounds() image.Rectangle
	At(x, y int) color.Color
	Set(x, y int, c color.Color)
	RGBA() *image.RGBA
}

type SurfaceFactory interface {
	New(width, height int) Surface
}

func DefaultSurfaceFactory() SurfaceFactory {
	return surfaceFactoryFunc(NewRGBASurface)
}

type surfaceFactoryFunc func(width, height int) Surface

func (f surfaceFactoryFunc) New(width, height int) Surface {
	return f(width, height)
}

// NewRGBASurface creates a Surface backed by image.RGBA.
func NewRGBASurface(width, height int) Surface {
	return &rgbaSurface{
		img: image.NewRGBA(image.Rect(0, 0, width, height)),
	}
}

// WrapRGBASurface exposes an existing *image.RGBA as a Surface.
func WrapRGBASurface(img *image.RGBA) Surface {
	if img == nil {
		return nil
	}
	return &rgbaSurface{img: img}
}

type rgbaSurface struct {
	img *image.RGBA
}

func (s *rgbaSurface) ColorModel() color.Model {
	return s.img.ColorModel()
}

func (s *rgbaSurface) Bounds() image.Rectangle {
	return s.img.Bounds()
}

func (s *rgbaSurface) At(x, y int) color.Color {
	return s.img.At(x, y)
}

func (s *rgbaSurface) Set(x, y int, c color.Color) {
	s.img.Set(x, y, c)
}

func (s *rgbaSurface) RGBA() *image.RGBA {
	return s.img
}

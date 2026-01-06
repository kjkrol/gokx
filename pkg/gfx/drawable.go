package gfx

import (
	"image/color"

	"github.com/kjkrol/gokg/pkg/plane"
)

type SpatialStyle struct {
	Fill   color.Color
	Stroke color.Color
}

type Drawable struct {
	ID uint64
	plane.AABB[uint32]
	Style SpatialStyle
	layer *Layer
}

func (d *Drawable) attach(layer *Layer) {
	d.layer = layer
}

func (d *Drawable) detach() {
	d.layer = nil
}

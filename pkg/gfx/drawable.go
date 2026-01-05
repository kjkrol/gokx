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
	plane.AABB[uint32]
	Style SpatialStyle
	layer *Layer
}

func (d *Drawable) Update(mutator func(shape *plane.AABB[uint32])) {
	if d == nil || mutator == nil {
		return
	}
	if d.layer == nil {
		mutator(&d.AABB)
		return
	}
	d.layer.ModifyDrawable(d, func() {
		mutator(&d.AABB)
	})
}

func (d *Drawable) attach(layer *Layer) {
	d.layer = layer
}

func (d *Drawable) detach() {
	d.layer = nil
}
